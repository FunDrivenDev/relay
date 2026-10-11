# Development

`just init` once per clone (the pinned tools, the Go modules),
then `just check` before pushing; `just --list` shows every recipe.

## Layout

| Path | Holds |
| --- | --- |
| `cmd/relay/main.go` | the program: the registry's entities, the version |
| `cmd/relay/script_test.go` | the functional test harness (testscript, `stub`, `exits`, a PATH of its own) |
| `internal/cli/` | the contract layer: specs, parser, JSON input, help, input spec, output, errors |
| `internal/fail/` | the error type and every error code (`codes.go`) |
| `internal/run/` | running external tools found on PATH |
| `internal/ui/` | Catppuccin Frappé styles, terminal detection |
| `internal/commands/commands.go` | the registry: one line per entity |
| `internal/<entity>/` | an entity's commands, one package each |
| `internal/fixture/` | the test-only `relay fixture` entity that exercises the contract |
| `testdata/script/*.txtar` | the functional tests, one or more per command |
| `docs/<entity>.md` | an entity's page, listed in `docs/README.md` |
| `scripts/*.sh` | the logic behind the Justfile recipes |

## Why the layer owns the parsing

cobra (on pflag) fights the contract on every rule that matters: pflag gives a
value flag the next argument even when it starts with `-` (the `--afk --yolo`
bug class), accepts `--bool=true`, lets a repeated flag silently win, and knows
nothing of `--json=<object>` building the same params. So `internal/cli`
parses on its own, about 400 lines driven by the spec, and Charm (lipgloss,
colorprofile, huh) styles what people read.

## Adding a command

1. **Params.** In `internal/<entity>/`, a struct with one field per input,
   tagged with its JSON key: `string`, `bool`, `int` or `[]string`.

   ```go
   type createParams struct {
       Name string   `json:"name"`
       Afk  bool     `json:"afk"`
       Args []string `json:"args"`
   }
   ```

2. **Spec and run function.** `cli.New` ties them; the layer checks the spec
   against the struct (a param without a field, a default of the wrong type…).

   ```go
   var create = cli.New(cli.Spec{
       Verb:    "create",
       Summary: "starts Claude Code under the relay, in a tmux session",
       Help:    "Starts Claude Code under the handover relay, in <dir>.",
       Output:  `{"session": {"name", "path", "afk", "where", "attached"}}`,
       Params: []cli.Param{
           {Name: "name", Help: "a named session that outlives the tab", Required: true},
           {Name: "afk", Help: "AFK run: unlimited relay until the goal is reached"},
           {Name: "args", Help: "the claude options"},
       },
       Positional:    "name",
       Rest:          "args",
       StateChanging: true, // adds --dry-run; read env.DryRun
   }, func(ctx context.Context, env *cli.Env, p createParams) (any, error) {
       if env.DryRun {
           return dryRunResult{…}, nil
       }
       out, err := run.Tool(ctx, "tmux", "new-session", "-d", "-s", p.Name)
       if err != nil {
           return nil, err // tool_missing or tool_failed
       }
       env.Say("session %s created", p.Name) // stderr, silent under --json
       return result{…}, nil
   })
   ```

   - Return a value: it is printed as JSON under `--json`; implement
     `Human(ui.Theme) string` on it for the human form (a `string` prints as
     is; anything else prints as JSON in both modes).
   - Return an error from `internal/fail`: `fail.New(fail.NotFound, "no session %s", name)`,
     `fail.Usagef(…)` for an input the spec cannot express, `fail.Quiet(code)`
     when the result already carries the verdict. A new code goes in
     `internal/fail/codes.go` and `docs/errors.md`.
   - `Interactive: true` for a command that needs a terminal; `Raw: true`
     (params `cli.Raw`) for one outside the contract (`session run`).

3. **Entity.** `internal/<entity>/<entity>.go`:

   ```go
   var Entity = &cli.Entity{Name: "session", Summary: "Claude Code sessions under tmux", Commands: []*cli.Command{create, list, attach}}
   ```

4. **Register it**: one line, sorted, in `internal/commands/commands.go`
   (`session.Entity,`).

5. **Document it**: `docs/<entity>.md`, and its line in `docs/README.md` turned
   into a link. `just lint` fails while either is missing.

6. **Test it**: `testdata/script/<entity>_<verb>.txtar` (below), driving the
   flag form and the JSON form and comparing them, the exit codes, the error
   envelope, `--help --json`, `--dry-run`. Unit tests next to the code for pure
   logic.

7. `just check`.

## Functional tests

`go test ./cmd/relay` runs every `testdata/script/*.txtar` with
[testscript](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript):
the test binary acts as the real `relay` (the fixture entity added), in a
fresh `$WORK` directory with `HOME=$WORK/home` (not created) and `NO_COLOR=1`.

Each script gets a PATH of its own: `$WORK/bin` (the stubs), `$WORK/sys`
(links to a few host programs the scripts need: `sh`, `bash`, `cat`, `tar`,
`sha256sum`…, `sysTools` in the harness), then `relay`. tmux, claude, python3,
ps, git, curl, sudo and uname are absent unless a script stubs them, so the
tests behave the same on a Mac and on Linux, whatever is installed. `$REPO` is
the repository root, for the scripts that run `install.sh` or `scripts/`.

Besides testscript's own commands (`exec`, `stdout`, `cmp`, `stdin`…), the
harness adds:

- `exits <status> <program> [args…]`: runs it and checks the exit status
  (`exits 2 relay x y --bogus`), its output then checked as `exec`'s.
- `stub <tool>…`: installs a fake tool in `$WORK/bin`, first on PATH. Each
  call appends its argv to `$WORK/stubs.log`, then prints
  `stubs/<tool>.stdout` and `stubs/<tool>.stderr` and exits with the number in
  `stubs/<tool>.exit` (0 without). `stubs/<tool>.<n>.<stream>` answers the
  n-th call only.

```txtar
stub tmux
exec relay session list --json
stdout '"name": "s1"'
cmp stubs.log want.log

-- stubs/tmux.stdout --
s1	1	0	/home/me/proj
-- stubs/tmux.2.exit --
1
-- want.log --
tmux list-sessions -F #{session_name}	#{session_windows}	#{session_attached}	#{pane_current_path}
```

`go test ./cmd/relay -update` rewrites the files a script compares with
`cmp` from the actual output: review the diff before committing it.

## Checks

- `just lint [scope]`: gofumpt, `go mod tidy -diff`, go vet, golangci-lint
  (`.golangci.yml`), the registry lint (every command has a summary, help and
  an input spec; every entity a docs page), rumdl, typos, shellcheck and shfmt
  (`scripts/*.sh`, `install.sh`), `just --fmt --check`, taplo, yamlfmt,
  `goreleaser check`.
- `just test [scope]`: `go test -race ./...` (without `-race` on staged files).
- `just audit`: gitleaks over the history (secrets, home-directory paths),
  govulncheck.

## Release

Linux only: amd64 and arm64 archives (`relay_linux_<arch>.tar.gz`, without
the version, so the latest release keeps one URL for `install.sh`),
`checksums.txt`, and `.deb` and `.rpm` packages (`.goreleaser.yaml`).

1. `just release` (snapshot) builds all of it into `dist/` without publishing.
2. On main, clean and green: `git tag vX.Y.Z && git push origin vX.Y.Z`.
3. `GITHUB_TOKEN=… just release publish` publishes the GitHub release of the
   tag.
4. `scripts/formula.sh X.Y.Z ><homebrew-tap>/Formula/relay.rb` writes the
   formula from `dist/checksums.txt`; open a PR on FunDrivenDev/homebrew-tap
   with it.

`install.sh` needs nothing more: it downloads the latest release.
