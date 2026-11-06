package fab

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// Run against a local server, never Fab or Discord. Install Playwright Firefox
// first and set FAB_BROWSER_TESTS=1; CI runs this suite explicitly.
func TestBrowserSourceIntegration(t *testing.T) {
	if os.Getenv("FAB_BROWSER_TESTS") != "1" {
		t.Skip("set FAB_BROWSER_TESTS=1 after installing Playwright Firefox")
	}
	source, err := NewBrowserSource()
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = source.Close() })
	source.Timeout = 5 * time.Second
	source.PostLoadDelay = 300 * time.Millisecond
	source.RenderTimeout = 5 * time.Second

	t.Run("heading and cards render after old fixed wait", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<html><body><main id="root"></main><script>
setTimeout(() => { document.querySelector('#root').innerHTML = '\x3csection>\x3ch2>Limited-Time Free\x3c/h2>\x3cul id="cards">\x3c/ul>\x3c/section>'; }, 1800);
setTimeout(() => { document.querySelector('#cards').innerHTML = '\x3cli>\x3ca href="/listings/late">Late asset\x3c/a>\x3c/li>'; }, 2400);
setTimeout(() => { document.querySelector('#cards').innerHTML += '\x3cli>\x3ca href="/listings/later">Later asset\x3c/a>\x3c/li>'; }, 2500);
</script></body></html>`)
		}))
		defer server.Close()
		source.URL = server.URL
		doc, err := source.Fetch(context.Background())
		if err != nil { t.Fatal(err) }
		result, err := ParseFreeAssets(doc)
		if err != nil || len(result.Assets) != 2 { t.Fatalf("assets=%v err=%v", result.Assets, err) }
	})

	t.Run("missing section times out with diagnostics", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `<html><title>Challenge page</title><h2>Other</h2></html>`)
		}))
		defer server.Close()
		source.URL = server.URL
		source.RenderTimeout = 150 * time.Millisecond
		_, err := source.Fetch(context.Background())
		if !errors.Is(err, ErrLimitedTimeFreeNotFound) || !strings.Contains(err.Error(), "status=200") || !strings.Contains(err.Error(), "Challenge page") {
			t.Fatalf("missing diagnostics: %v", err)
		}
	})

	t.Run("HTTP errors are not parsed as empty pages", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, emptyFreeSection)
		}))
		defer server.Close()
		source.URL = server.URL
		_, err := source.Fetch(context.Background())
		if err == nil || !strings.Contains(err.Error(), "status=503") { t.Fatalf("got %v", err) }
	})

	t.Run("cancellation interrupts navigation", func(t *testing.T) {
		entered := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(entered)
			<-r.Context().Done()
		}))
		defer server.Close()
		source.URL = server.URL
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() { _, err := source.Fetch(ctx); done <- err }()
		select {
		case <-entered:
		case <-time.After(10 * time.Second): t.Fatal("navigation did not start")
		}
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) { t.Fatalf("got %v", err) }
		case <-time.After(3 * time.Second): t.Fatal("cancellation did not interrupt navigation")
		}
	})
}
