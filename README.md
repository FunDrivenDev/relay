# relay

Claude Code sessions under tmux with the
[handover plugin](https://github.com/FunDrivenDev/claude-plugins)'s relay, for
Linux (Ubuntu first): interactive sessions that hand over to a fresh context
when theirs fills up, AFK runs that keep going toward a goal, and YOLO mode.

relay drives `claude`; the handover plugin (`handover@fundrivendev`) does the
handovers. **The plugin is required**: `relay doctor fix` installs it.

## Install

Linux, amd64 or arm64. Pick one:

```sh
# Homebrew on Linux
brew install fundrivendev/tap/relay

# The install script: no sudo, into ~/.local/bin; run it again to update.
curl -fsSL https://raw.githubusercontent.com/FunDrivenDev/relay/main/install.sh | sh

# The .deb of a release (it pulls tmux, python3 and procps); relay_0.1.0_arm64.deb on arm64
curl -fsSLO https://github.com/FunDrivenDev/relay/releases/download/v0.1.0/relay_0.1.0_amd64.deb
sudo apt install ./relay_0.1.0_amd64.deb
```

Each release also has `.rpm` packages (`relay-0.1.0-1.x86_64.rpm`,
`relay-0.1.0-1.aarch64.rpm`) and the archives the install script takes
(`relay_linux_amd64.tar.gz`, `relay_linux_arm64.tar.gz`).

The install script checks the download's sha256 against the release's
`checksums.txt`. `RELAY_VERSION=0.1.0` installs another release than the
latest, `RELAY_INSTALL_DIR=<dir>` another directory than `~/.local/bin`, and
`RELAY_BASE_URL=<url>` downloads the archive and `checksums.txt` from
`<url>` instead of GitHub (a mirror, or a `just release` build's `dist/`
served over HTTP).

## Check the machine

```sh
relay doctor check   # read-only: Linux, tmux >= 3.2, claude, python3 >= 3.9,
                     # ps, git, notify-send, the handover plugin, notifications
relay doctor fix     # installs what is missing, after asking
```

`doctor fix` runs `sudo apt-get update` and one `sudo apt-get install -y` line
for the missing packages,
Claude Code's installer (`curl -fsSL https://claude.ai/install.sh | bash`), then
`claude plugin marketplace add FunDrivenDev/claude-plugins` and
`claude plugin install handover@fundrivendev`. `--dry-run` shows the plan,
`--yes` skips the question.

`git`, `notify-send` and `notifications` are optional (see
[Notifications](#notifications)). Ubuntu 20.04's tmux 3.0 and Python 3.8 are
too old: use 22.04 or later. See [docs/doctor.md](docs/doctor.md).

## Use

```sh
relay session create                         # Claude Code here, in a tmux session tied to this tab
relay session create --name api --dir ~/src/api 'review the open TODOs'
relay session create --name fix-ci --afk --until 8 --yolo 'make CI green'
relay session list                           # the tmux sessions
relay session attach fix-ci                  # back to a session
relay session create --name fix-ci --afk 'make CI green' --dry-run   # what would run
```

`--afk <goal>` runs unattended: when a session's context fills up, the plugin
writes a handover and starts the next session on it, until one says
`AFK done:` or `AFK blocked:`, or the `--until` deadline (`8`, `tomorrow 8`,
`+10h`) passes. A named AFK session outlives the terminal: detach with tmux,
come back with `relay session attach`.

> **YOLO** (`--yolo`, AFK only) runs Claude Code with
> `--dangerously-skip-permissions`: it acts without asking you. The plugin adds
> guardrails and an audit log, but the agent can still run any command your
> user can. Use it in a disposable checkout or a VM, never with credentials you
> cannot afford to leak.

The handover plugin's settings (where handovers are written, the notifier)
are in its own docs.

## Notifications

The plugin calls you back when an AFK run ends, waits or stops: a desktop
notification on macOS and on a Linux desktop (`notify-send`, package
`libnotify-bin`), and the terminal bell always. Headless, over SSH or under
WSL, only the bell rings: set the plugin's `notify_command` to reach you
elsewhere (Slack, ntfy).

```sh
relay notifier check   # the environment detected, the adapters, the hints
relay notifier test    # sends a test alert through them
```

For Slack, keep `notifier` on `auto` and point `notify_command` at a script
that posts to a webhook (in `/handover:config`, or Claude Code's
`settings.json`):

```json
{"pluginConfigs": {"handover@fundrivendev": {"options": {
  "notifier": "auto",
  "notify_command": "$HOME/.local/bin/handover-notify-slack"
}}}}
```

The script, the webhook setup and ntfy are in the plugin's
[notifications guide](https://github.com/FunDrivenDev/claude-plugins/blob/main/plugins/handover/docs/notifications.md);
relay's side is in [docs/notifier.md](docs/notifier.md).

Every command takes `--help`, `--json` input and output, and `--dry-run` when
it changes something: see [the docs](docs/README.md).

## Licence

[MIT](LICENSE).
