# Exit and error codes

## Exit status

| Status | Meaning |
| --- | --- |
| 0 | success |
| 1 | failure: the command ran and did not succeed |
| 2 | usage error: the command line or the JSON input is wrong |

## The error envelope

Under `--json` (in any form: `--json`, `--json=<object>`, `--json=-`), an error
is one line of JSON on stderr, and nothing else goes to stderr:

```json
{"error":{"code":"not_found","message":"no session foo"}}
```

Without `--json`, the error is one line for people: `relay: <message>`.

A usage error's message ends with the help to read: `(relay <entity> <verb> --help)`.

A command may print its result on stdout and still fail (`doctor check`
reports every check, then exits 1). A *quiet* failure prints no error line
without `--json`: the result already carries the verdict (`"ok": false`) and
the exit status says the rest.

## Codes

Codes are stable: an agent branches on them, so a code is never renamed or
given another meaning. They are declared in `internal/fail/codes.go`; a test
checks this table lists every one, and only those.

| Code | Exit | Meaning |
| --- | --- | --- |
| `usage` | 2 | unknown, repeated or malformed option, unknown JSON key, wrong type, missing required input |
| `failure` | 1 | an error without a more precise code |
| `exists` | 1 | what the command would create already exists |
| `interrupted` | 1 | the command was cancelled (Ctrl-C, SIGTERM) |
| `not_found` | 1 | what the command acts on does not exist |
| `tool_failed` | 1 | an external tool (tmux, python3, claude…) exited with an error; the message names the command and the first line of its stderr |
| `tool_missing` | 1 | an external tool is not on PATH |
| `tty_required` | 1 | an interactive command was run under `--json`, or a confirmation found no terminal; the message gives what to run instead |
| `relay_missing` | 1 | session: the handover plugin is not installed (`handover.py` not found through `installed_plugins.json`) |
| `declined` | 1 | `doctor fix`: the confirmation was answered no; nothing ran |
| `fix_failed` | 1 | `doctor fix`: a fix command failed; the ones after it did not run, the checks were run again and reported |
| `requirements_missing` | 1 | `doctor check`, `doctor fix`: a required check failed; the report on stdout lists it and its fix |
| `not_sent` | 1 | `notifier test`: the test alert got through no notifier, not even the terminal bell; the report on stdout says why |
