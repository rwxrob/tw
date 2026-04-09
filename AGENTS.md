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

`cmd-tw` is a Go CLI tool (module `github.com/rwxrob/tw`) built with [bonzai](https://github.com/rwxrob/bonzai).

### Commands

- `tw` — root command; delegates to subcommands
- `tw serve` — self-backgrounding HTTP server (port 8080); subcommands: `stop`, `tail`
- `tw topic` — set Twitch stream title + auto-detect category from keywords
- `tw category` — interactively pick and set Twitch category
- `tw clips` — manage/sync Twitch clips
- `tw what` — show current stream info
- `tw obs` — OBS WebSocket helpers; subcommand `rtirl` (was `add-rtirl`)
- `tw token` (was `cache-token`) — interactive OAuth token refresh via `twitch token -u`

All commands include `help.Cmd.AsHidden()` as the first entry in their `Cmds` slice.

### Key packages

- `internal/twitch/` — shared Twitch Helix API helpers
  - `LoadCreds()` — reads `TWITCH_CLIENT_ID`/`TWITCH_TOKEN` env vars or parses `~/.twitch-cli.env`
  - `BroadcasterID()` — HTTP GET `/helix/users`
  - `GetCategory()`, `Title()` — HTTP GET `/helix/channels`
  - `PatchChannels(broadcasterID, jsonBody)` — HTTP PATCH `/helix/channels`
  - `LoadCategories()`, `MatchCategory()` — YAML category matching
- `serve/` — daemon lifecycle, HTTP overlay server, Twitch poller, OBS/Belabox pollers
- `obs/` — OBS WebSocket scene helpers

### Daemon

`tw serve` self-backgrounds (parent forks, exits; child runs). PID written to `~/.local/share/tw/serve.pid`. Logs to `~/Library/Logs/tw.log`. No launchd — run directly.

### Twitch API

All Twitch API calls use direct HTTP to `api.twitch.tv/helix/*` — no `twitch` CLI subprocess calls (except `tw token` which intentionally uses `twitch token -u` for interactive OAuth).
Credentials: `~/Library/Application Support/twitch-cli/.twitch-cli.env` (macOS) with `CLIENTID` and `ACCESSTOKEN` keys.

### Bonzai conventions

- Command names: no dashes (bonzai routing breaks)
- `Cmd.Short`: ≤50 runes, must begin with lowercase letter
- `help.Cmd.AsHidden()` always first in every `Cmds` slice

## Current tags / versions

- bonzai: v0.56.7
- Go module: `github.com/rwxrob/tw`
