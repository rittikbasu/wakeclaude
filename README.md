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
4. Enter the prompt to run.
5. Pick when to run it (one-time, daily, weekly).

Controls:

- Arrow keys to move, `enter` to select, type to search
- `esc` to go back, `q` to quit
- Prompt entry: `ctrl+d` to continue

Schedule input:

- One-time: enter date + time
- Daily: enter time (24-hour)
- Weekly: pick day + enter time (24-hour)

Output:

- JSON describing the selection, prompt, and schedule.

Example:

```json
{"projectPath":"/Users/me/dev/app","sessionId":"...","sessionPath":"/Users/me/.claude/projects/.../id.jsonl","newSession":false,"model":"opus","prompt":"...","schedule":{"type":"weekly","weekday":"Tuesday","time":"09:00","timezone":"Local"}}
```

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
