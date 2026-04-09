# tw — Twitch streaming automation CLI

A bonzai-based CLI tool for managing a Twitch livestream setup including OBS scene switching, Belabox IRL stats, clip serving, and stream metadata.

## Install

```sh
go install github.com/rwxrob/tw@latest
```

## Commands

| Command | Alias | Description |
|---------|-------|-------------|
| `tw` | `w` | Show current topic and Twitch category (default) |
| `tw what` | `w` | Show current topic and Twitch category |
| `tw topic [keyword\|-]` | `t` | Get/set stream topic; `-` swaps to previous |
| `tw category [keyword]` | `cat`, `c` | Pick/set Twitch stream category |
| `tw serve` | | Start the HTTP/WebSocket daemon |
| `tw clips` | | Manage local Twitch clip files |
| `tw obs` | | OBS helpers |
| `tw cachetoken` | | Cache a Twitch OAuth token |

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
| `TWITCH_BROADCASTER_ID` | _(auto-discovered)_ | Twitch broadcaster ID |
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
| `TWITCH_BROADCASTER_ID` | _(auto-discovered via `twitch api get /users`)_ | Twitch broadcaster ID |
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
