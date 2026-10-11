# session

Claude Code sessions under tmux and the handover plugin's relay
(`handover@fundrivendev`, from the FunDrivenDev/claude-plugins marketplace).
`run` is the relay itself, formerly the plugin's `scripts/claude-relay`.
Every command follows [the CLI contract](contract.md), except `run`, which is
raw (below). `relay doctor check` tells whether this machine has what they
need.

| Command | Does |
| --- | --- |
| `relay session create` | starts Claude Code under the relay, in a tmux session |
| `relay session list` | lists the sessions |
| `relay session attach` | attaches to a session |
| `relay session run` | runs Claude Code with the handover relay on |

## session create

```text
relay session create [--name <name>] [--dir <dir>] [--afk [--until <when>] [--yolo]]
                       [--detach] [--dry-run] [<prompt>] [-- <claude options>…]
relay session create --json='{"name": "…", "afk": true, "prompt": "…", …}'
```

Starts `relay session run` with the relay arguments below, in `<dir>` (the
current directory by default).

| Option | JSON key | Meaning |
| --- | --- | --- |
| `--name <name>` | `name` | a named session that outlives the tab; refused (`exists`) when `<name>` exists |
| `--dir <dir>` | `dir` | where Claude Code starts (`not_found` when it is not a directory) |
| `<prompt>`, `--prompt` | `prompt` | the first message; the goal with `--afk` |
| `--afk` | `afk` | AFK run: unlimited relay until the goal is reached or blocked; requires `<prompt>` |
| `--until <when>` | `until` | AFK deadline: `8`, `tomorrow 8`, `mon 9:00`, `+10h`; requires `--afk` |
| `--yolo` | `yolo` | AFK run without permission prompts, with guardrails and an audit log; requires `--afk` |
| `--detach` | `detach` | creates the named session without attaching; implied by `--json`, requires `--name` |
| `-- <claude options>…` | `args` | the options for claude |
| `--dry-run` | `dry_run` | validates and reports what would run, without running it |

Where Claude Code runs, the output's `where`:

| `where` | When | What happens |
| --- | --- | --- |
| `detached` | `--detach` or `--json` | `tmux new-session -d -s <name> -c <dir> relay session run …`, `destroy-unattached off` |
| `attached` | `--name`, outside tmux | the same session, attached on this terminal (relay gives way to tmux) |
| `current` | `--name`, under tmux | the current tmux session is renamed, `destroy-unattached off`, `HANDOVER_RELAY_OWN` unset in it; the relay runs in this pane |
| `pane` | no `--name`, under tmux | the relay runs in this pane |
| `tab` | no `--name`, outside tmux | the relay starts a tmux session tied to the tab ([run](#session-run)) |

The relay arguments (`relay_args`) are, in this order: `--afk <goal>
[--until <when>] [--yolo]`, the claude options, then the prompt when not an
AFK run. The flag form and the JSON form give the same ones.

Output:

```json
{"session": {"name": "s1", "path": "/home/me/proj", "afk": false, "where": "detached", "attached": false}}
```

With `--dry-run`, nothing runs, and the handover plugin need not be installed:

```json
{"dry_run": true, "session": {"name": "s1", "path": "/home/me/proj", "afk": true, "where": "detached"},
 "relay_args": ["--afk", "goal", "--until", "8", "--yolo"]}
```

Without `--json`, a message goes to stderr (`session s1 created (relay
session attach s1)`, or the dry run's `would run (<where>) in <dir>: relay
session run …`) and nothing to stdout.

Errors: `usage` (exit 2: `--afk` without a prompt, `--until` or `--yolo`
without `--afk`, `--detach` or `--json` without `--name`, the contract's own),
`not_found` (no such directory), `exists` (the session exists),
`relay_missing` (the handover plugin is not installed), `tool_failed` and
`tool_missing` (tmux).

```sh
relay session create --name fix-ci --afk --until 8 --yolo 'make CI green' -- --model opus
relay session create --json='{"name": "s1", "prompt": "hello", "dir": "~/Code/x"}'
relay session create --name s1 --dry-run --json
```

## session list

```text
relay session list [--json]
```

The tmux sessions: name, windows, whether a client is attached, the current
path of its active pane. On a colour terminal, a table; when stdout is not a
terminal or `NO_COLOR` is set, one plain line each (`s1  1 window(s)
(attached)  /home/me/proj`), for scripts. No tmux server means no session
(`no sessions` on stderr). `--json` is the same in every case.

```json
{"sessions": [{"name": "s1", "windows": 1, "attached": false, "path": "/home/me/proj"}]}
```

## session attach

```text
relay session attach <name>
```

Attaches to session `<name>`: `tmux attach-session -t =<name>`, or
`tmux switch-client -t =<name>` under tmux. Interactive: under `--json` it is
refused with `tty_required`. Errors: `not_found` (no such session), `usage`.

## session run

```text
relay session run [<claude args>…]
relay session run --afk <goal> [--until <when>] [--yolo] [<claude args>…]
```

Claude Code with the handover relay on: once a handover is ready and the
session has written its closing reply, a fresh session resumes from it without
the user, up to the plugin's `relay_limit` handovers. It is what `session
create` starts.

It is **raw**: its callers fix its interface and its arguments mix its own
options with claude's, so it takes them untouched (no `--json`, no
`--dry-run`; `--help` and `--help --json` are read). `--afk`, `--until` and
`--yolo` may come anywhere; their value is the next argument, whatever it
starts with; every other argument goes to claude in its order.

- `--afk <goal>`: an AFK run toward `<goal>`, which is also the first prompt:
  no `relay_limit`; the plugin sends sessions back to work until one ends a
  reply on `AFK done: <why>` or `AFK blocked: <why>`.
- `--until <when>`: no new handover past the deadline. It is resolved once,
  by `handover.py until <when>`, and passed on as `@<epoch>` (given as
  `@<epoch>`, it is taken as is).
- `--yolo`: claude runs with `--dangerously-skip-permissions --settings
  <json>`, the settings of `handover.py yolo-settings`, merged into the user's
  own `--settings` (`--merge`): claude takes a single `--settings`.

Where it runs:

1. Outside tmux, with tmux installed, it starts itself again inside a new tmux
   session, in the current directory: an AFK run in a named session
   `afk-<directory>` (`.` and `:` written `-`; `-2`, `-3`… while taken), which
   outlives the tab and is announced on stderr; any other in a session tied to
   the tab (`tmux new-session -e HANDOVER_RELAY_OWN=1`).
2. Inside tmux, it sets `destroy-unattached` on its pane's session (`off` for
   an AFK run, `on` when `HANDOVER_RELAY_OWN` is set) and gives way to claude;
   the relay types `/clear` and the resume message into the pane.
3. Without tmux, it loops: the relay leaves the resume message in
   `$HANDOVER_RELAY_FILE` and ends claude, and the loop starts claude again
   with the same arguments plus that message (an AFK run's goal is the first
   prompt only), until a session ends without one. Ctrl-C belongs to claude.

The environment it sets for claude is the contract with the plugin's
`handover.py`: [handover-relay.md](handover-relay.md).

Exit status: 2 for `--until` or `--yolo` without `--afk`, or an option
without its value; 1 for `relay_missing` (`--until` or `--yolo` without the
plugin installed), `tool_failed` (`handover.py` refused the deadline or the
settings), `tool_missing` (python3, claude); otherwise claude's own, or 0 when
the loop ends.

## Environment

| Variable | Read by | Meaning |
| --- | --- | --- |
| `CLAUDE_CONFIG_DIR` | create, run | Claude Code's config directory (default `~/.claude`): its `plugins/installed_plugins.json` gives `.plugins["handover@fundrivendev"][0].installPath`, whose `scripts/handover.py` is run with `python3` |
| `TMUX`, `TMUX_PANE` | create, attach, run | under tmux, and the pane to configure |
| `HANDOVER_RELAY_OWN` | run | see [handover-relay.md](handover-relay.md) |
