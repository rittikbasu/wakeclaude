# wakeclaude

a tiny macos tui to schedule claude code prompts so your sessions can keep running even when you hit limits or go to sleep.

https://github.com/user-attachments/assets/d8d489d8-e676-47eb-a8bc-f54c677c0405

## why i built it

i hit the 5 hour session rate limit on my claude plan a lot. work would stop mid‑flow, so i wanted a way to auto‑resume right when the limit resets.

now i can schedule a prompt to continue at the reset time, and i can also schedule prompts while i’m sleeping so the work keeps going. one thing i do weekly is run a security review on my codebases just before the weekly rate limit resets (while i’m asleep).

## what it does

- pick a project, pick a session (or start a new one)
- write the prompt
- choose a model + permission mode
- schedule it (one‑time, daily, weekly)
- wakes your mac only when needed and runs the prompt
- keeps logs + shows a simple run history
- sends a native macos notification on success/error

## install (homebrew)

```bash
brew install --cask rittikbasu/wakeclaude/wakeclaude
```

to update later:

```bash
brew update
brew upgrade --cask wakeclaude
```

## quickstart

1. build it:

   ```bash
   go build -o wakeclaude ./cmd/wakeclaude
   ```

2. run it:

   ```bash
   ./wakeclaude
   ```

## setup token (required)

wakeclaude uses a long‑lived claude code token so scheduled prompts keep working even after you close the terminal.

generate one in a separate terminal:

```bash
claude setup-token
```

paste it into wakeclaude when prompted. it stores the token in your **macos keychain** (not in files).

## how it works (macos)

- uses **launchd** (launchdaemons) to run on schedule
- uses **pmset schedule wakeorpoweron** to wake the mac only when needed
- you’ll be prompted for sudo when creating/editing/deleting schedules
- the job runs as root, then uses `launchctl asuser` to run `claude` in your user session

important: if you are fully logged out, `claude` may not be able to access your keychain session. running while asleep with the user still logged in works best.

## usage (tui)

you’ll see a simple menu:

- **schedule a prompt** (project → session → prompt → model → permission → time)
- **manage scheduled prompts** (edit/delete)
- **view run logs**

controls:

- arrow keys to move, `enter` to select
- type to search (projects, sessions, schedules, logs)
- `esc` to go back, `q` to quit
- prompt entry: `ctrl+d` to continue

## models + permission modes

models:
- `opus`
- `sonnet`
- `haiku`

permission modes:
- `acceptEdits` – auto‑accept file edits + filesystem access
- `plan` – read‑only, no commands or file changes
- `bypassPermissions` – skips permission checks (use with care)

## logs + notifications

data lives here:

- `~/Library/Application Support/WakeClaude/schedules.json`
- `~/Library/Application Support/WakeClaude/logs.jsonl`
- `~/Library/Application Support/WakeClaude/logs/*.log`

run logs are retained (last 50) and shown in the tui. each run also triggers a native macos notification (via `osascript`).

## flags

- `--projects-root <path>`: override default `~/.claude/projects`
- `--run <id>`: internal (used by launchd)

## assumptions

- claude code sessions live under `~/.claude/projects`
- root‑level `.jsonl` files are sessions
- project display names are derived from the session `cwd` when available
- `claude` must be in your PATH at scheduling time (wakeclaude records it)

## contributing

open a pr if you want — bug fixes, ux polish, or smarter scheduling ideas are welcome.
