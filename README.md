# Fab Free Asset Discord DM

GitHub Actions checks Fab every six hours and sends newly listed **Limited-Time Free** assets to multiple Discord users by DM.

## Setup

1. Create a Discord application and bot in the Discord Developer Portal.
2. Add the bot to a server you share with it, then enable Developer Mode in Discord and copy your user ID.
3. In this repository, add `DISCORD_BOT_TOKEN` and `DISCORD_USER_IDS` Actions secrets. Put every recipient ID in `DISCORD_USER_IDS`, separated by commas. The older `DISCORD_USER_ID` secret is still supported.
4. Open **Actions → Fab free asset DM → Run workflow** once to test the live Fab lookup and DM.

The workflow stores every notified Fab listing URL in `data/seen-assets.json`, so scheduled runs do not send duplicates. If the Discord API call fails, the state is not advanced and the next run retries it.

## Schedule

The workflow runs at minute 17 every six hours. GitHub schedules use UTC and may start a little late during busy periods.

## Local tests

```bash
go test ./...
```

## License

MIT

### Scraper reliability and regression tests

The browser checks the navigation HTTP status, then polls rendered HTML using the
same Limited-Time Free section parser as the scraper. Navigation has a 60-second
timeout; rendering has a separate 30-second window. Populated sections must remain
unchanged for 1.5 seconds before collection. A heading without cards is allowed the
full render window before it is treated as an empty promotion. This settling window
is a heuristic: later changes outside it can still require tuning. Missing sections
and sections that never settle fail with status, final URL, page title, and HTML
length diagnostics. These distinguish response/redirect/challenge symptoms from
missing markup, but cannot establish the cause of historical failures.

The notifier makes at most five attempts with 5, 10, 20, and 40-second waits.
Cancellation stops navigation, render polling, and retry waits. Exhaustion preserves
the final error and attempt count. The browser is closed after collection.

Run unit and retry regressions with:

```sh
go test -race ./...
```

Run Firefox regressions with delayed JavaScript served by a local test server:

```sh
go run github.com/mxschmitt/playwright-go/cmd/playwright@v0.6201.1 install --with-deps firefox
FAB_BROWSER_TESTS=1 go test -race ./internal/fab -run TestBrowserSourceIntegration -count=1 -v
```

The browser fixture adds the heading after 1.8 seconds and cards after 2.4/2.5
seconds, reproducing content that the old fixed 1.5-second wait missed. Other tests
cover HTTP 503, missing sections, cancellation, empty sections, card changes,
recovery after three failures, and retry exhaustion. The PR test workflow runs
these tests without Discord credentials or production notifications.
