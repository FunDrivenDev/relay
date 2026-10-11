# notifier

How the handover plugin's alerts reach you: when the relay halts, when an AFK
run ends, waits on a prompt or stops on an error. The plugin
(`handover@fundrivendev` 0.16.0 or later) detects the environment and picks
the adapters; relay reads its report, `python3 handover.py notifier`, and
never detects anything itself. Both commands follow
[the CLI contract](contract.md). The plugin's options, and how to set up Slack
or ntfy: [the plugin's notifications guide](https://github.com/FunDrivenDev/claude-plugins/blob/main/plugins/handover/docs/notifications.md).

| Command | Does |
| --- | --- |
| `relay notifier check` | reports the environment, the adapters, whether they reach you, the hints |
| `relay notifier test` | sends a test alert through them |

Claude Code sees the environment of its tmux session: a session started from a
desktop terminal has its display, one started over SSH has none, even when you
attach to it from the desktop later. Run these where the sessions start.

## notifier check

```text
relay notifier check
relay notifier check --json
```

Read-only; exit 0 whatever it reports.

```text
environment  linux, headless, over SSH (over SSH, without a display)
notifier     auto (notify_command not set)
adapters     bell
! only the terminal bell rings: alerts may not reach you
  over SSH, without a display: only the bell rings; set notify_command (Slack, ntfy…) to be reached
  options: /handover:config, …; guide: https://github.com/FunDrivenDev/claude-plugins/…
```

The output is the plugin's report as it gives it:

```json
{
  "environment": {"os": "linux", "desktop": false, "ssh": true,
                  "why": "over SSH, without a display",
                  "tools": {"osascript": false, "notify-send": false}},
  "notifier": "auto",
  "notify_command_set": false,
  "adapters": ["bell"],
  "reaches_you": false,
  "hints": ["over SSH, without a display: only the bell rings; set notify_command (Slack, ntfy…) to be reached", "options: …; guide: …"]
}
```

| Key | Meaning |
| --- | --- |
| `environment.os` | `macos`, `linux`, `wsl` (notify-send reaches no Windows desktop) or `other` |
| `environment.desktop` | a desktop notification can reach a screen here; `why` says what decided it |
| `environment.ssh` | the session runs over SSH |
| `notifier` | the plugin's `notifier` option: `auto`, `macos`, `linux`, `command`, `bell` |
| `notify_command_set` | whether `notify_command` is set (never its value) |
| `adapters` | what an alert goes through, the terminal bell last |
| `reaches_you` | false when only the bell rings |
| `hints` | what to install or set, from the plugin |

## notifier test

```text
relay notifier test [--dry-run]
relay notifier test --json='{"dry_run": true}'
```

Runs `handover.py notifier --test`: a test alert through every adapter, the
bell included. The output is the report above with `dry_run` and `sent`.
`--dry-run` sends nothing (`handover.py notifier`, without `--test`). A bell
alone counts as sent: `reaches_you` says whether more than the bell rang.

| Code | When |
| --- | --- |
| `not_sent` | no adapter got through, not even the bell; the report is on stdout |
| `relay_missing` | the handover plugin is not installed |
| `tool_missing` | python3 is not on PATH |
| `tool_failed` | handover.py failed, or its report is unreadable (a plugin older than 0.16.0) |

## In relay doctor check

`relay doctor check` runs the same report for two optional checks, which never
fail the doctor: `notifications` (ok when the alerts reach you; the plugin's
hints and this guide's link as its advice; skipped without the plugin or
python3), and `notify-send`, which only a Linux desktop uses: elsewhere
(headless, SSH, WSL, macOS) its absence is fine, and the advice is
`notify_command`, not `libnotify-bin`. See [doctor.md](doctor.md).
