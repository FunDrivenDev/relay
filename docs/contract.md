# The CLI contract

Every relay command follows one contract, so a person and an agent drive it
the same way. The contract layer (`internal/cli`) implements it once, from each
command's declaration: no command parses its own arguments.

## Shape

- `relay <entity> <verb> [options] [<positional>] [-- <passthrough>…]`: noun
  then verb, no shorthand. An entity may group verbs under a second noun
  (`relay <entity> <group> <verb>`); `relay <entity> <group>` prints the
  group's verbs.
- `relay`, `relay --help`, `relay help` print the list of commands;
  `relay <entity>` prints its verbs. Both exit 0, on stdout.
- `relay --version` prints `relay <version>` (`--json`: `{"version"}`).

## Input: flags

- Options in any order, before or after the positional argument.
- **At most one positional argument**: the object acted on, or free text. A
  command whose positional is a list takes several. The
  positional can also be given as its option (`--name foo`), not both.
- **A value option takes the next argument, never one starting with `-`**:
  write `--opt=-value` for that. `--count -1` is a usage error; `--count=-1` is
  not.
- **An optional value is given only through `=`**: `--color` alone takes the
  option's implicit value, `--color=never` another; in `--color never`, `never`
  is the positional argument.
- **A boolean takes no value**: `--force=true` is a usage error.
- **A repeated or unknown option is a usage error**, so is `--dry_run` (options
  are kebab-case) and any argument beyond the one positional.
- A list option takes comma-separated values: `--tags=a,b`.
- One-letter aliases exist only where a command declares one (`-n` for
  `--count` in the test fixture); `-h` is `--help` everywhere.
- **Everything after `--` passes through untouched**, to the command that
  declares a passthrough param (for instance the arguments of a program it
  runs); elsewhere `--` is a usage error.

## Input: JSON

- `--json=<object>`, or `--json=-` to read it from stdin, gives the whole
  input as one JSON object. Its keys are the long option names in snake_case
  (`--dry-run` → `dry_run`); the positional's key is its option's, the
  passthrough's key is the param's name (`args`).
- It builds the same values as the flags, then the same code checks them
  (required, enum, defaults), binds them to the command's params and runs it.
- Types are strict: `string`, `boolean`, `integer` (a JSON number without a
  fraction), `array` (of strings). An unknown key, a wrong type, a second JSON
  value after the object are usage errors.
- Mixing `--json=<object>` with any option or argument is a usage error.

## Output

- **stdout carries data only**; messages for people go to stderr, and none
  under `--json`.
- `--json` (or `--json=<input>`) makes the output JSON, indented, one document.
  Each command's help describes its shape (the `output` key of its spec).
- Without `--json`, the human form: styled in Catppuccin Frappé on a terminal
  (tables drawn with lipgloss), plain lines when stdout is not a terminal or
  `NO_COLOR` is set.

## Behaviour

- **Every state-changing command takes `--dry-run`** (`dry_run`): it validates
  its input and reports what it would do, without doing it. Other commands
  reject it as an unknown option.
- **An interactive command refuses under `--json`** with `tty_required`, its
  message giving the command line to run in a terminal (the same input, in the
  flag form). Usage errors come first. One whose `--dry-run` asks nothing
  opts in (`DryRunJSON` in its spec) to run that dry run under `--json`.
- Exit 0 on success, 1 on failure, 2 on a usage error; under `--json` an error
  is `{"error":{"code","message"}}` on stderr ([errors.md](errors.md)).

## Help and the input spec

`relay <entity> <verb> --help` prints the help; `--help` wins over any other
argument (before `--`). With `--json`, it prints the command's input spec,
here the one of the test-only `fixture echo` command:

```json
{
  "command": "relay fixture echo",
  "summary": "prints its input back",
  "input": {"text": "string", "count": "integer", "tags": "array", "loud": "boolean", "args": "array", "dry_run": "boolean"},
  "positional": "text",
  "rest": "args",
  "short": {"n": "count"},
  "required": ["text"],
  "defaults": {"count": 1},
  "enum": {"mode": ["plain", "upper"]},
  "implicit": {"color": "auto"},
  "help": {"text": "the text to print", "…": "…"},
  "output": "{\"text\", \"count\", …}",
  "state_changing": true,
  "interactive": false
}
```

| Key | Holds |
| --- | --- |
| `command` | the command line that runs it |
| `summary` | one line |
| `input` | each JSON key and its type, in the help's order |
| `positional` | the key the positional argument fills, if any |
| `rest` | the key that takes the arguments after `--`, if any |
| `short` | one-letter aliases, letter → key |
| `required` | the keys that must be given |
| `defaults` | the value of a key left out |
| `enum` | the accepted values of a key |
| `implicit` | the value of an optional-value option given alone |
| `help` | each key's help |
| `output` | the shape of the JSON output |
| `state_changing` | takes `dry_run` |
| `interactive` | refused under `--json` |
| `dry_run_json` | interactive, but its `--dry-run` runs under `--json` (present only when true) |
| `raw` | outside the contract (below) |

`state_changing` and `interactive` are always there; the other keys are absent
when empty.

`relay --help --json` (and `relay <entity> --help --json`) prints the
index: `{"commands": [{"command", "summary"}]}`.

## Outside the contract: raw commands

A program whose caller fixes the interface is declared `Raw`: it receives its
arguments untouched, `--json` and `--` included. `relay session run` is one:
its arguments mix its own options with claude's, and it re-executes itself
inside tmux with them. Only a lone
`--help` (or `--help --json`) is read by the layer, so the command stays
documented. A raw command takes no `--dry-run`.
