# Agent Instructions

Keep this project as agent-agnostic as possible.

## Rule: Context Maintenance

At the end of every significant task or session, summarize the current state, architectural decisions made, and pending "todo" items into AGENTS.md. Always ensure this file reflects the "ground truth" of the project so future sessions can resume without friction. Use the writeFile tool to overwrite it so the next session starts with current state.

## Rule: Commits

- Always use conventional commits (e.g. `feat:`, `fix:`, `docs:`, `chore:`)
- Never add anything agent related (copilot, claude, etc.) to commit messages or co-authorship
- Committing directly to main is okay in this repo

## Rule: Environment

- Use `/usr/bin/open` (full path) to open files or URLs on macOS — never plain `open`

## Rule: Secrets

- Never commit secrets, config files, or database files

## Rule: Code style

- Single-line paragraphs in all markdown files — no multi-line wrapped paragraphs
- No underscores or spaces in filenames; use hyphens
- No extensions on executable scripts, ever

## Rule: Agent specific

- Always use `gh copilot` not `copilot`

## Current architecture

`tw` is a Go CLI tool (module `github.com/rwxrob/tw`) built with [bonzai](https://github.com/rwxrob/bonzai).

### Commands

- `tw` — root command; delegates to subcommands
- `tw serve` — self-backgrounding HTTP server (port 8080); subcommands: `stop`, `restart`, `tail`
- `tw topic` — set Twitch stream title + auto-detect category from keywords
- `tw category` — interactively pick and set Twitch category
- `tw clips` — manage/sync Twitch clips; subcommand `set dir <path>` to configure clips directory
- `tw what` — show current stream info
- `tw obs` — OBS WebSocket helpers; subcommand `rtirl`
- `tw login` — interactive OAuth user token flow via `twitch token -u -s channel:manage:broadcast`
- `tw var` — manage bonzai vars (get/set/data/edit/delete/grep); warns on read ops if interactive

All commands include `help.Cmd.AsHidden()` as the first entry in their `Cmds` slice.

### Key packages

- `internal/twitch/` — shared Twitch Helix API helpers
  - `LoadCreds()` — reads credentials from env vars, bonzai vars, or twitch-cli env file (priority order)
  - `CategoriesFile()` — reads path from env/vars/default
  - `client()` — returns `golang.org/x/oauth2`-backed `*http.Client` + clientID; auto-refreshes token using stored `REFRESHTOKEN`
  - `BroadcasterID() (string, error)` — HTTP GET `/helix/users` (no params; user token returns authenticated user)
  - `ChannelTitle(broadcasterID string) (string, error)` — HTTP GET `/helix/channels`
  - `GetCategory()` — HTTP GET `/helix/channels`
  - `PatchChannels(broadcasterID, jsonBody)` — HTTP PATCH `/helix/channels`
  - `LoadCategories()`, `MatchCategory()` — YAML category matching
- `internal/serve/` — daemon lifecycle, HTTP overlay server, Twitch poller, OBS/Belabox pollers
- `internal/obs/` — OBS WebSocket scene helpers
- `internal/login/` — OAuth flow via `twitch token -u`, filters sensitive output, verifies broadcaster ID

All cmd packages live under `internal/` to prevent external imports. Only `main.go` is at the root.

### Daemon

`tw serve` self-backgrounds (parent forks, exits; child runs). PID written to `~/.local/share/tw/serve.pid`. Logs to `~/Library/Logs/tw.log`. No launchd — run directly.

### Configuration via bonzai vars

All user-configurable values are read via `vars.Fetch[T](envVar, key, fallback)` — env vars still take priority, then bonzai vars, then hardcoded defaults.
Stored at `~/.local/state/tw/vars.properties`.
Use `tw var set <Key> <value>` to configure, or `tw clips set dir <path>` for the clips directory.

Var keys: `ClipsDir`, `TopicsFile`, `Port`, `OBSWSAddr`, `OBSPasswordFile`, `OBSLiveScene`, `OBSClipsScene`, `RTIRLKeyFile`, `LiveSceneFile`, `BelaboxStatsURLFile`, `LogFile`, `PIDFile`, `CategoriesFile`, `TwitchClientID`, `TwitchToken`.

### Twitch API

All Twitch API calls use direct HTTP to `api.twitch.tv/helix/*` via `golang.org/x/oauth2` client — no `twitch` CLI subprocess calls except `tw login` which intentionally uses `twitch token -u` for the interactive OAuth browser flow.
Credentials: `~/Library/Application Support/twitch-cli/.twitch-cli.env` (macOS). Keys: `CLIENTID`, `CLIENTSECRET`, `ACCESSTOKEN`, `REFRESHTOKEN`.
A **user token** (not app token) is required — `channel:manage:broadcast` scope needed for PATCH /channels. Run `tw login` to authenticate.

### Bonzai conventions

- Command names: no dashes (bonzai routing breaks)
- `Cmd.Short`: ≤50 runes, must begin with lowercase letter
- `help.Cmd.AsHidden()` always first in every `Cmds` slice

## Current tags / versions

- bonzai: v0.56.7
- golang.org/x/oauth2: v0.36.0
- Go module: `github.com/rwxrob/tw`
- Latest tag: v0.1.6
