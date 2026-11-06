package fab

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrHomepageNotReady = errors.New("fab: homepage render timed out")

// waitForFreeSection uses the same parser as the scraper, so a heading outside
// a section or unrelated listing cards cannot satisfy readiness. A populated
// section must stop changing for settle; a heading alone is not ready. Preserve
// empty-section support only after allowing the entire render window for cards.
// This is a bounded settling heuristic, not proof that all site JS has finished.
func waitForFreeSection(ctx context.Context, read func() (string, error), timeout, settle, poll time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var previous string
	var stableSince time.Time
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		document, err := read()
		if err != nil {
			return "", fmt.Errorf("fab: read homepage HTML: %w", err)
		}
		section, _, found := locateFreeSection(document)
		result, err := ParseFreeAssets(document)
		if err != nil {
			return document, err
		}
		now := time.Now()
		if !found || section != previous {
			previous, stableSince = section, now
		}
		if found && len(result.Assets) > 0 && now.Sub(stableSince) >= settle {
			return document, nil
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			if found && len(result.Assets) == 0 {
				return document, nil
			}
			if !found {
				return document, fmt.Errorf("%w after %s: %w", ErrHomepageNotReady, timeout, ErrLimitedTimeFreeNotFound)
			}
			return document, fmt.Errorf("%w after %s: section did not settle (%d cards)", ErrHomepageNotReady, timeout, len(result.Assets))
		}
		if remaining < poll {
			poll = remaining
		}
		if err := sleepContext(ctx, poll); err != nil {
			return document, err
		}
	}
}
