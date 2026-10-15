# Fab Free Asset Discord DM

GitHub Actions checks Fab every six hours and sends newly listed **Limited-Time Free** assets to one Discord user by DM.

## Setup

1. Create a Discord application and bot in the Discord Developer Portal.
2. Add the bot to a server you share with it, then enable Developer Mode in Discord and copy your user ID.
3. In this repository, add Actions secrets named `DISCORD_BOT_TOKEN` and `DISCORD_USER_ID`.
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
