# The handover relay contract

`relay session run` (formerly the handover plugin's `scripts/claude-relay`)
starts Claude Code; the plugin's `handover.py`, run by Claude Code's hooks,
drives the relay. The two live in different repos (FunDrivenDev/relay and
FunDrivenDev/claude-plugins) and meet only through what is below: the
environment `session run` gives claude, and the `handover.py` subcommands it
calls. Changing either side means changing both; the functional tests
`testdata/script/session_run*.txtar` pin relay's side.

This contract is shared, byte for byte, with the private `toolbox session run`
it was ported from: the same plugin works under either CLI.

## Finding handover.py

`.plugins["handover@fundrivendev"][0].installPath` in
`${CLAUDE_CONFIG_DIR:-~/.claude}/plugins/installed_plugins.json`, then
`<installPath>/scripts/handover.py`, run as `python3 <path> …`. It is looked up
on every call: the path changes with every plugin version. Not found, the
command fails with `relay_missing` ([errors.md](errors.md)).

## The environment

| Variable | Set by | Read by | Meaning and values |
| --- | --- | --- | --- |
| `HANDOVER_RELAY` | `session run`, always: `1` | `handover.py` `relay_on()` (the relay is on for this session) and `relay_start()` (`stats` reports `start=claude-relay`) | the session was started under the relay; `1` or unset |
| `HANDOVER_RELAY_OWN` | `session run`, outside tmux, on the tmux session it starts for a run that is not AFK (`tmux new-session -e HANDOVER_RELAY_OWN=1`); `session create` empties it for a named session (`set-environment -u` on the renamed session, `HANDOVER_RELAY_OWN=` for the relay it runs) | `session run`, inside tmux | the tmux session belongs to the relay and goes with its tab: `destroy-unattached on`. `1`, or empty or unset (the session outlives the tab). `handover.py` does not read it |
| `HANDOVER_RELAY_FILE` | `session run`, outside tmux (loop mode): a fresh temporary file `claude-relay.*` in `$TMPDIR`, removed when the loop ends | `handover.py` `relay()`: writes the resume message there, then has `relay-drive --kill` end claude; `cmd_afk`: AFK mode is refused unless `TMUX_PANE` or this is set | the next prompt. The loop starts claude again while the file is not empty, with that message (trailing newlines removed) as the last argument, then empties it |
| `HANDOVER_RELAY_PID` | `session run`, loop mode: its own pid | `handover.py` `claude_pid()`: walks up the `ps` ancestry to the process whose parent has this pid | the loop's pid; claude is its direct child |
| `HANDOVER_AFK_GOAL` | `session run --afk <goal>`, inside tmux and in loop mode | `handover.py` SessionStart hook (startup and clear): starts the AFK run | the goal; also claude's first prompt, its last argument |
| `HANDOVER_AFK_UNTIL` | `session run --until`, with `--afk` | `handover.py` SessionStart hook: the run's deadline | Unix epoch in seconds, without the `@` |
| `HANDOVER_AFK_YOLO` | `session run --yolo`, with `--afk`: `1` | `handover.py` SessionStart hook (stored in the run's AFK state); any user hook that treats a YOLO run apart | a YOLO run; `1` or unset |

`TMUX` and `TMUX_PANE` come from tmux: inside tmux, `handover.py` types
`/clear` and the resume message into `$TMUX_PANE`, and `session run` sets
`destroy-unattached` on it. `HANDOVER_CLAUDE_PID` and `HANDOVER_CHILD` are
internal to `handover.py`.

Outside tmux, `session run` with tmux installed sets none of these but
`HANDOVER_RELAY` for its own `tmux new-session`: it starts itself again inside
that session, where it sets them. The arguments it passes on are its relay
options, the deadline already resolved (`--until @<epoch>`), then claude's.

## The handover.py subcommands

| Call | When | Relied on |
| --- | --- | --- |
| `handover.py until <when>` | `session run --until <when>`, unless `<when>` starts with `@`; once, before any tmux session | the epoch on stdout, alone on its line; a line for people on stderr (`AFK until …`), passed on; exit 1 on a deadline it cannot read (relay: `tool_failed`) |
| `handover.py yolo-settings [--merge <settings>]` | `session run --afk … --yolo`, inside tmux or in loop mode; `--merge` with the value of the user's own `--settings` (JSON or a file path), taken out of claude's arguments | a settings JSON object on stdout (with `permissions.deny` rules), passed as `claude --dangerously-skip-permissions --settings <json>`; exit 2 on bad `--merge` input (relay: `tool_failed`) |

## The plugin's side

The plugin's `scripts/claude-relay` may stay as a shim, `exec relay session run
"$@"` (or `toolbox session run`), failing when neither is installed.
