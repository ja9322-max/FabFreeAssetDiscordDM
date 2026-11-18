package fab

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type sourceFunc func(context.Context) (string, error)
func (f sourceFunc) Fetch(ctx context.Context) (string, error) { return f(ctx) }

func TestScraperRecoversAfterThreeFailures(t *testing.T) {
	calls, delays := 0, 0
	s := NewScraper(sourceFunc(func(context.Context) (string, error) {
		calls++
		switch calls {
		case 1: return "", context.DeadlineExceeded
		case 2: return `<html>loading</html>`, nil
		case 3: return "", ErrHomepageNotReady
		default: return populatedFreeSection, nil
		}
	}))
	s.Backoff = func(attempt int) time.Duration { delays++; if attempt != delays { t.Fatalf("attempt=%d", attempt) }; return 0 }
	assets, _, err := s.GetFreeAssets(context.Background())
	if err != nil || len(assets) != 1 || calls != 4 || delays != 3 {
		t.Fatalf("assets=%v err=%v calls=%d delays=%d", assets, err, calls, delays)
	}
}

func TestScraperExhaustionPreservesCause(t *testing.T) {
	for _, cause := range []error{nil, errors.New("HTTP 503"), ErrHomepageNotReady} {
		calls, delays := 0, 0
		s := NewScraper(sourceFunc(func(context.Context) (string, error) { calls++; return "<html>loading</html>", cause })).WithRetries(3)
		s.Backoff = func(int) time.Duration { delays++; return 0 }
		_, _, err := s.GetFreeAssets(context.Background())
		want := cause
		if want == nil { want = ErrLimitedTimeFreeNotFound }
		if !errors.Is(err, want) || !strings.Contains(err.Error(), "3 attempts") || calls != 3 || delays != 2 {
			t.Fatalf("err=%v calls=%d delays=%d", err, calls, delays)
		}
	}
}

func TestScraperCancellation(t *testing.T) {
	for _, phase := range []string{"before fetch", "fetch", "backoff"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			s := NewScraper(sourceFunc(func(context.Context) (string, error) {
				calls++
				if phase == "fetch" { cancel() }
				return "", ErrHomepageNotReady
			}))
			s.Backoff = func(int) time.Duration { cancel(); return time.Hour }
			if phase == "before fetch" { cancel() }
			_, _, err := s.GetFreeAssets(ctx)
			wantCalls := 1
			if phase == "before fetch" { wantCalls = 0 }
			if !errors.Is(err, context.Canceled) || calls != wantCalls { t.Fatalf("err=%v calls=%d", err, calls) }
		})
	}
}

func TestScraperEmptySectionDoesNotRetry(t *testing.T) {
	calls := 0
	s := NewScraper(sourceFunc(func(context.Context) (string, error) { calls++; return emptyFreeSection, nil }))
	assets, _, err := s.GetFreeAssets(context.Background())
	if err != nil || len(assets) != 0 || calls != 1 { t.Fatalf("assets=%v err=%v calls=%d", assets, err, calls) }
}

func TestDefaultBackoffIsBounded(t *testing.T) {
	for i, want := range []time.Duration{5, 10, 20, 40, 60, 60} {
		if got := defaultBackoff(i+1); got != want*time.Second { t.Fatalf("attempt %d: got %s", i+1, got) }
	}
}
