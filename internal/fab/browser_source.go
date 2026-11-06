package fab

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/mxschmitt/playwright-go"
)

const browserOverrideEnv = "ASSETS_BOT_BROWSER"

const defaultBrowserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:144.0) Gecko/20100101 Firefox/144.0"

const antiBotScript = `Object.defineProperty(navigator, 'webdriver', {get: () => undefined});
Object.defineProperty(navigator, 'platform', {get: () => 'Win32'});
Object.defineProperty(navigator, 'language', {get: () => 'en-US'});
Object.defineProperty(navigator, 'languages', {get: () => ['en-US', 'en']});`

type BrowserSource struct {
	BrowserPath   string
	URL           string
	UserAgent     string
	Timeout       time.Duration
	RenderTimeout time.Duration
	// PostLoadDelay is the minimum time the populated section must remain stable.
	PostLoadDelay time.Duration

	mu      sync.Mutex
	pw      *playwright.Playwright
	browser playwright.Browser
}

func NewBrowserSource() (*BrowserSource, error) {
	browserPath, err := resolveBrowserOverride()
	if err != nil {
		return nil, err
	}

	return &BrowserSource{
		BrowserPath:   browserPath,
		URL:           defaultHomepageURL,
		UserAgent:     defaultBrowserUserAgent,
		Timeout:       60 * time.Second,
		RenderTimeout: 30 * time.Second,
		PostLoadDelay: 1500 * time.Millisecond,
	}, nil
}

func (s *BrowserSource) Fetch(ctx context.Context) (document string, err error) {
	if s == nil {
		return "", fmt.Errorf("fab: browser source is not initialized")
	}

	if err := ctx.Err(); err != nil {
		return "", err
	}

	browserPath := strings.TrimSpace(s.BrowserPath)
	if browserPath == "" {
		resolved, resolveErr := resolveBrowserOverride()
		if resolveErr != nil {
			return "", resolveErr
		}
		browserPath = resolved
	}

	url := strings.TrimSpace(s.URL)
	if url == "" {
		url = defaultHomepageURL
	}

	userAgent := strings.TrimSpace(s.UserAgent)
	if userAgent == "" {
		userAgent = defaultBrowserUserAgent
	}

	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	postLoadDelay := s.PostLoadDelay
	if postLoadDelay <= 0 {
		postLoadDelay = 1500 * time.Millisecond
	}

	renderTimeout := s.RenderTimeout
	if renderTimeout <= 0 {
		renderTimeout = 30 * time.Second
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}

	browser, err := s.ensureBrowserLocked(browserPath)
	if err != nil {
		return "", err
	}

	browserContext, err := browser.NewContext(playwright.BrowserNewContextOptions{
		Locale:    playwright.String("en-US"),
		UserAgent: playwright.String(userAgent),
		Viewport: &playwright.Size{
			Width:  1440,
			Height: 900,
		},
	})
	if err != nil {
		s.resetLocked()
		return "", fmt.Errorf("fab: create firefox context: %w", err)
	}
	// Closing the per-fetch context interrupts Playwright calls on cancellation.
	var closeOnce sync.Once
	closeContext := func() { closeOnce.Do(func() { _ = browserContext.Close() }) }
	stopCancel := context.AfterFunc(ctx, closeContext)
	defer func() {
		stopCancel()
		closeContext()
		if ctx.Err() != nil {
			document, err = "", ctx.Err()
		}
	}()

	if err := browserContext.AddInitScript(playwright.Script{
		Content: playwright.String(antiBotScript),
	}); err != nil {
		s.resetLocked()
		return "", fmt.Errorf("fab: add anti-bot init script: %w", err)
	}

	page, err := browserContext.NewPage()
	if err != nil {
		s.resetLocked()
		return "", fmt.Errorf("fab: create firefox page: %w", err)
	}

	log.Printf("Loading homepage...")
	response, err := page.Goto(url, playwright.PageGotoOptions{
		Timeout:   playwright.Float(float64(timeout.Milliseconds())),
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
	})
	if err != nil {
		return "", fmt.Errorf("fab: navigate homepage: %w", err)
	}
	if response == nil {
		return "", fmt.Errorf("fab: homepage navigation returned no HTTP response (url=%q)", page.URL())
	}
	status := response.Status()
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("fab: homepage HTTP status=%d url=%q", status, page.URL())
	}

	document, err = waitForFreeSection(ctx, page.Content, renderTimeout, postLoadDelay, 250*time.Millisecond)
	if err != nil {
		title, _ := page.Title()
		return "", fmt.Errorf("fab: wait for rendered homepage (status=%d url=%q title=%q bytes=%d): %w",
			status, page.URL(), title, len(document), err)
	}

	return document, nil
}

func (s *BrowserSource) Close() error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var result error
	if s.browser != nil {
		if err := s.browser.Close(); err != nil && result == nil {
			result = fmt.Errorf("fab: close firefox: %w", err)
		}
		s.browser = nil
	}
	if s.pw != nil {
		if err := s.pw.Stop(); err != nil && result == nil {
			result = fmt.Errorf("fab: stop playwright: %w", err)
		}
		s.pw = nil
	}
	return result
}

func (s *BrowserSource) ensureBrowserLocked(browserPath string) (playwright.Browser, error) {
	if s.browser != nil {
		return s.browser, nil
	}

	pw, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("fab: start playwright: %w", err)
	}

	launchOptions := playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
	}
	if browserPath != "" {
		launchOptions.ExecutablePath = playwright.String(browserPath)
	}

	browser, err := pw.Firefox.Launch(launchOptions)
	if err != nil {
		_ = pw.Stop()
		return nil, fmt.Errorf("fab: launch firefox: %w", err)
	}

	s.pw = pw
	s.browser = browser
	return browser, nil
}

func (s *BrowserSource) resetLocked() {
	if s.browser != nil {
		_ = s.browser.Close()
		s.browser = nil
	}
	if s.pw != nil {
		_ = s.pw.Stop()
		s.pw = nil
	}
}

func resolveBrowserOverride() (string, error) {
	override := strings.TrimSpace(os.Getenv(browserOverrideEnv))
	if override == "" {
		return "", nil
	}

	if resolved, ok := resolveExecutable(override); ok {
		return resolved, nil
	}

	return "", fmt.Errorf("fab: firefox executable %q from %s was not found", override, browserOverrideEnv)
}

func resolveExecutable(candidate string) (string, bool) {
	if strings.TrimSpace(candidate) == "" {
		return "", false
	}

	if strings.Contains(candidate, string(os.PathSeparator)) {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
		return "", false
	}

	resolved, err := exec.LookPath(candidate)
	if err != nil {
		return "", false
	}
	return resolved, true
}
