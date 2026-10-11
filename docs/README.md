# relay docs

- [The CLI contract](contract.md): how every command reads its input and writes its output.
- [Exit and error codes](errors.md).
- [Development](development.md): layout, adding a command, tests, release.

## Entities

One page per entity, one line each here.

- [doctor](doctor.md): what relay needs on this machine, checked and installed, `doctor check|fix`
- [notifier](notifier.md): how the handover plugin's alerts reach you, `notifier check|test`
- [session](session.md): Claude Code sessions under tmux and the handover relay, `session create|list|attach|run`; its contract with the handover plugin: [handover-relay.md](handover-relay.md)
