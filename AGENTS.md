# relay

A Go CLI for Linux: Claude Code sessions under tmux with the handover plugin's
relay (`relay session …`), and what they need on the machine
(`relay doctor …`). Public repository: no personal paths, names or secrets.

Checks: just check

- Read [docs/development.md](docs/development.md) before changing code: the
  layout, how a command is added, the test harness.
- Every command follows [the CLI contract](docs/contract.md); every entity has
  its page under `docs/`, linked from [docs/README.md](docs/README.md).
- The settings relay passes to the handover plugin are a contract shared with
  other callers: [docs/handover-relay.md](docs/handover-relay.md). Change
  them only together with the plugin.
- Tests never run the real tools: tmux, claude, python3, ps, git, curl, sudo
  and uname are stubbed (`stub <tool>`) in `testdata/script/*.txtar`.
- Everything is written in English. Commits follow Conventional Commits.
