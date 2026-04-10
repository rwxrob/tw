# tw — Twitch streaming automation CLI

A bonzai-based CLI tool for managing a Twitch livestream setup including OBS scene switching, Belabox IRL stats, clip serving, and stream metadata.

## Install

Requires [Go](https://go.dev/dl/) 1.21+:

```sh
go install github.com/rwxrob/tw@latest
```

Pre-built binaries via goreleaser are planned for a future release.

## Commands

| Command | Description |
|---------|-------------|
| `tw` | Show current stream topic and Twitch category (default) |
| `tw what` | Show current stream topic and Twitch category |
| `tw topic [keyword\|-]` | Get/set stream topic; fuzzy-matches existing topics; auto-updates Twitch title and category |
| `tw category [keyword]` | Pick or set Twitch stream category; fuzzy-matches by name |
| `tw serve` | Start all daemons in background (HTTP, OBS, Twitch, Belabox, clips) |
| `tw serve stop` | Stop the running daemon |
| `tw serve restart` | Stop and restart the running daemon |
| `tw serve tail` | Tail the daemon log file |
| `tw clips` | List downloaded Twitch clips |
| `tw clips sync` | Sync clips from Twitch |
| `tw obs rtirl` | Add RTIRL map browser source to OBS scene |
| `tw login` | Authenticate with Twitch via OAuth (user token, `channel:manage:broadcast` scope) |

## Authentication

`tw login` is required before first use and whenever the token expires.
It wraps `twitch token -u -s channel:manage:broadcast` to obtain a **user access token** stored in the twitch-cli config file (`~/Library/Application Support/twitch-cli/.twitch-cli.env` on macOS).

A **user token** (not an app token) is required because:
- `GET /helix/users` without query params only works with a user token (returns the authenticated user's info)
- `PATCH /helix/channels` requires a user token with `channel:manage:broadcast` scope

The token auto-refreshes via the stored refresh token using `golang.org/x/oauth2` — no manual re-login needed until the refresh token itself expires.

## Configuration (vars)

All configuration is stored in `~/.local/state/tw/vars.properties` as `key=value` pairs managed via `tw var`.

⚠️ **This file contains sensitive credentials** (`TwitchClientID`, `TwitchToken`) — keep it private and never commit it.

Use `tw var set <key> <value>` to configure, `tw var edit` to open in `$EDITOR`, and `tw var data` to view all values.

## Topics file

Topics are stored one per line in `~/.topics` (override with `TOPICS` or `TOPIC` env var). The first line is the current topic. `tw topic -` swaps in the previous topic.

When a topic is set, `tw topic` automatically updates the Twitch stream title and picks the matching category from the categories file.

## Categories file

Categories live in `~/.config/tw/categories.yaml` (override with `TWITCH_CATEGORIES_FILE`).

A `categories-sample.yaml` is included in the repo — copy it to get started:

```sh
mkdir -p ~/.config/tw
cp categories-sample.yaml ~/.config/tw/categories.yaml
```

YAML is used so that regex patterns require no escaping — `\b` stays `\b` rather than needing `\\b` as in JSON, and most patterns need no quoting at all since YAML plain scalars handle `|`, `.`, `*`, and `()` without special treatment.

Twitch game `id` values are permanent — Twitch assigns them once and never changes them, so they are safe to hardcode.

Format — an ordered list; first regex match wins:

```yaml
- regex: shop|bike|road|climb|hike|walk|camp|with fixie
  name: IRL
  id: 509672

- regex: hot.?tub|jacuzzi|pool.*soak|soak.*pool
  name: Pools, Hot Tubs, and Beaches
  id: 116747788

- regex: to fixie|(build|fixing|fixed|tune|repair|wrench).*(bike|bicycle)
  name: Makers & Crafting
  id: 509673

- regex: yoga
  name: Fitness & Health
  id: 509671

- regex: writ
  name: Writing & Reading
  id: 772157971

- regex: club|coffee|☕|break
  name: Just Chatting
  id: 509658

- regex: beginner.boost|cowork|conf.*call|schedule
  name: Co-working & Studying
  id: 1599346425

- regex: cod(e|ing)
  name: Software and Game Development
  id: 1469308723

- regex: hack|linux
  name: Science & Technology
  id: 509670
```

Regexes are matched case-insensitively against the full topic string.

## Environment variables

### `tw serve` daemon

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |
| `TOPICS` / `TOPIC` | `~/.topics` | Path to topics file |
| `CLIPS_DIR` | `~/Videos/twclips` | Directory of local clip files |
| `CLIPS_BITRATE_THRESHOLD` | `600` | Belabox kbps threshold to consider stream live |
| `BELABOX_POLL` | `2` | Belabox stats poll interval (seconds) |
| `TWITCH_POLL` | `60` | Twitch API poll interval (seconds) |
| `CLIPS_SYNC_INTERVAL` | `3600` | Clip sync interval (seconds) |
| `OBS_CLIPS_SCENE` | `Clips` | OBS scene name for clip playback |
| `OBS_LIVE_SCENE_FILE` | `~/.local/state/tw-live-scene` | File tracking active OBS scene |
| `OBS_WS_PASSWORD_FILE` | `~/.config/obs-websocket/password` | OBS WebSocket password file |
| `BELABOX_STATS_URL_FILE` | `~/.config/tw/belabox-stats-url` | File containing Belabox stats URL |

### CLI commands

| Variable | Default | Description |
|----------|---------|-------------|
| `TOPICS` / `TOPIC` | `~/.topics` | Path to topics file |
| `TWITCH_CATEGORIES_FILE` | `~/.config/tw/categories.yaml` | Path to categories YAML file |

## Overlays

The `tw serve` daemon exposes browser-source overlays:

| Path | Description |
|------|-------------|
| `/overlay` | OBS stream info overlay (topic + category) |
| `/clips` | Current clip player |
| `/clips/retrotv` | Clip player styled as a retro CRT television |

## Belabox polling

Belabox stats are only polled when the active OBS scene starts with `IRL` or matches `OBS_CLIPS_SCENE`. This avoids unnecessary network traffic during non-IRL scenes.
