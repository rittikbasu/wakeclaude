# wakeclaude

wakeclaude is a small TUI for finding and resuming local Claude Code sessions.

## Usage

Run it with no arguments:

```sh
wakeclaude
```

It opens an interactive picker:

1. Pick a project (search box at the top).
2. Pick a session, or choose **Start a new session**.
3. Pick a Claude model to use.

Controls:

- Arrow keys to move, `enter` to select, type to search
- `esc` to go back, `q` to quit

Output:

- Existing session: `UUID<TAB>SESSION_PATH<TAB>MODEL`
- New session: `new<TAB>PROJECT_PATH<TAB>MODEL`

Models:

- `auto` (default)
- `sonnet`, `opus`, `haiku`

Flags:

- `--projects-root <path>`: override the default `~/.claude/projects`

## Assumptions

- Claude Code sessions live under `~/.claude/projects`.
- Root-level `.jsonl` files are sessions.
- Project display names are derived from the session file `cwd` when available.

## Build

```sh
go build -o wakeclaude ./cmd/wakeclaude
```
