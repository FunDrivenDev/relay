# doctor

What relay needs on this machine, checked (`doctor check`, read-only) and
installed (`doctor fix`). relay is made for Linux, and the fixes are Ubuntu's
(apt-get); elsewhere the checks still run, the `os` check warns, and only the
Claude Code and plugin fixes are offered. Both
commands follow [the CLI contract](contract.md).

| Command | Does |
| --- | --- |
| `relay doctor check` | checks the requirements, each with the command that fixes it |
| `relay doctor fix` | runs those commands, after asking |

## The checks

| Check | Required | Met when | Fix |
| --- | --- | --- | --- |
| `os` | no | Linux; the distribution is read from `/etc/os-release` | none: a warning |
| `tmux` | yes | on PATH, 3.2 or later | `sudo apt-get install -y tmux` |
| `claude` | yes | Claude Code on PATH | `curl -fsSL https://claude.ai/install.sh \| bash` |
| `python3` | yes | on PATH, 3.9 or later | `sudo apt-get install -y python3` |
| `ps` | yes | procps' `ps`, taking `-o ppid= -p 1` | `sudo apt-get install -y procps` |
| `git` | no | on PATH | `sudo apt-get install -y git` |
| `notify-send` | no | on PATH | `sudo apt-get install -y libnotify-bin` |
| `handover-plugin` | yes | `handover@fundrivendev` in Claude Code's `plugins/installed_plugins.json` | `claude plugin marketplace add FunDrivenDev/claude-plugins && claude plugin install handover@fundrivendev` |

Why these:

- **tmux 3.2**: `relay session create` passes the relay's variables with
  `tmux new-session -e`, which came in tmux 3.2 (Ubuntu 22.04 ships 3.2a;
  20.04 ships 3.0a, too old). A tmux below the floor has no automatic fix:
  upgrade the distribution or build tmux.
- **Python 3.9**: the handover plugin's `handover.py` uses
  `str.removeprefix`. Ubuntu 20.04's 3.8 is too old, with the same advice.
- **ps**: `handover.py` walks the process tree with `ps -o ppid= -p <pid>`.
  busybox's `ps` does not take it; procps' does.
- **git**, optional: `handover.py` reads the repository for the handover's
  context; without git it writes the handover all the same.
- **notify-send**, optional: the handover plugin's Linux notifier sends AFK
  alerts (a run that stops, waits or ends) as desktop notifications. Without
  it, or with no desktop (over SSH), the alerts fall back to the terminal
  bell. Ubuntu's package is `libnotify-bin`.
- **handover-plugin**: relay runs the plugin's `scripts/handover.py`, found
  through `installed_plugins.json` under `$CLAUDE_CONFIG_DIR` (else
  `~/.claude`).

The optional checks (`os`, `git`, `notify-send`) never fail the doctor; they
show as `!`. A `claude` in `~/.local/bin` while that directory is off PATH
fails the `claude` check with no fix but the advice to add it to the shell's
profile: tmux runs `claude` through PATH, and reinstalling would not help.

The tests stand in for the host with three variables: `RELAY_GOOS` (the OS),
`RELAY_OS_RELEASE` (the os-release file) and `RELAY_EUID` (the effective user
id, `0` for root).

## doctor check

```text
relay doctor check
relay doctor check --json
```

Prints a checklist: `✓` met, `✗` a required check failed, `!` an optional one,
each unmet check followed by its fix. Exits 1 with code
`requirements_missing` when a required check fails; under `--json` the report
is still printed on stdout, and the error envelope goes to stderr.

```json
{
  "ok": false,
  "os": "linux",
  "distro": "Ubuntu 24.04.1 LTS",
  "checks": [
    {"name": "tmux", "ok": false, "required": true, "version": "",
     "message": "not found on PATH", "fix": "sudo apt-get install -y tmux"}
  ],
  "fixes": [
    {"name": "packages", "command": "sudo apt-get install -y tmux"}
  ]
}
```

`fix` is `null` when there is none. `fixes` is the plan `doctor fix` runs,
in order:

1. `packages`: one `apt-get install -y` line with every missing package
   (`sudo` omitted when relay runs as root), plus `curl` when Claude Code is
   missing and curl is too;
2. `claude`: Claude Code's installer;
3. `marketplace`: `claude plugin marketplace add FunDrivenDev/claude-plugins`,
   only when Claude Code does not know the `fundrivendev` marketplace
   (`plugins/known_marketplaces.json`);
4. `handover-plugin`: `claude plugin install handover@fundrivendev`.

## doctor fix

```text
relay doctor fix [--yes] [--dry-run]
relay doctor fix --json='{"yes": true}'
```

Shows each command before running it with `sh -c`, on stderr. On a terminal
it first lists the plan and asks; `--yes` (`-y`) skips the question. Without
a terminal and without `--yes` it is refused (`tty_required`); `--json`
without `--yes` or `--dry-run` is a usage error, since it cannot ask.
`--dry-run` prints the plan and runs nothing.

The first failing command stops the fix (`fix_failed`), the checks run again and the report says where it stands. Claude Code's
installer puts `claude` in `~/.local/bin`: relay adds it to its own PATH for
the plugin steps, and says to add it to the shell's when it is missing. The
checks run again at the end: a required one still failing ends on
`requirements_missing`.

```json
{
  "ok": true,
  "dry_run": false,
  "steps": [
    {"name": "packages", "command": "sudo apt-get install -y tmux",
     "ran": true, "ok": true, "error": ""}
  ],
  "checks": ["… as doctor check, after the fix"]
}
```

| Code | When |
| --- | --- |
| `requirements_missing` | a required check fails (after the fix, for `fix`) |
| `fix_failed` | a fix command failed; the following ones did not run |
| `declined` | the question was answered no |
| `tty_required` | no terminal to ask on, and no `--yes` |
| `usage` | `--json` without `--yes` or `--dry-run` |
