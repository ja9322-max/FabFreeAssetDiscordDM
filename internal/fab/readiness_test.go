package fab

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

const emptyFreeSection = `<section><h2>Limited-Time Free</h2></section>`
const populatedFreeSection = `<section><h2>Limited-Time Free</h2><ul><li><a href="/listings/one">One</a></li></ul></section>`

func TestWaitForFreeSectionDelayedRendering(t *testing.T) {
	start := time.Now()
	read := func() (string, error) {
		switch {
		case time.Since(start) < 20*time.Millisecond:
			return `<section><h2>Other</h2></section>`, nil
		case time.Since(start) < 70*time.Millisecond:
			return emptyFreeSection, nil
		default:
			return populatedFreeSection, nil
		}
	}
	doc, err := waitForFreeSection(context.Background(), read, time.Second, 20*time.Millisecond, time.Millisecond)
	if err != nil || doc != populatedFreeSection {
		t.Fatalf("accepted missing/heading-only section: doc=%q err=%v", doc, err)
	}
}

func TestWaitForFreeSectionWaitsForCardChanges(t *testing.T) {
	start := time.Now()
	complete := strings.Replace(populatedFreeSection, "</ul>", `<li><a href="/listings/two">Two</a></li></ul>`, 1)
	read := func() (string, error) {
		if time.Since(start) < 20*time.Millisecond {
			return populatedFreeSection, nil
		}
		return complete, nil
	}
	doc, err := waitForFreeSection(context.Background(), read, time.Second, 50*time.Millisecond, time.Millisecond)
	if err != nil || doc != complete || time.Since(start) < 70*time.Millisecond {
		t.Fatalf("did not wait for stable cards: doc=%q err=%v", doc, err)
	}
}

func TestWaitForFreeSectionTimeouts(t *testing.T) {
	for _, tc := range []struct {
		name string
		doc string
		want error
	}{
		{"missing", `<h2>Limited-Time Free</h2>`, ErrLimitedTimeFreeNotFound},
		{"unsettled", populatedFreeSection, ErrHomepageNotReady},
		{"empty", emptyFreeSection, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			doc, err := waitForFreeSection(context.Background(), func() (string, error) { return tc.doc, nil }, 30*time.Millisecond, time.Second, time.Millisecond)
			if !errors.Is(err, tc.want) || doc != tc.doc || time.Since(start) < 30*time.Millisecond {
				t.Fatalf("doc=%q err=%v duration=%s", doc, err, time.Since(start))
			}
		})
	}
}

func TestWaitForFreeSectionCancellationAndReadError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	readErr := errors.New("browser disconnected")
	_, err := waitForFreeSection(ctx, func() (string, error) {
		cancel()
		return "", nil
	}, time.Minute, time.Second, time.Millisecond)
	if !errors.Is(err, context.Canceled) { t.Fatalf("got %v", err) }
	_, err = waitForFreeSection(context.Background(), func() (string, error) { return "", readErr }, time.Minute, time.Second, time.Millisecond)
	if !errors.Is(err, readErr) { t.Fatalf("got %v", err) }
}
