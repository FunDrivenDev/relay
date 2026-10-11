package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"

	"github.com/FunDrivenDev/relay/internal/commands"
	"github.com/FunDrivenDev/relay/internal/fixture"
)

// update rewrites the files a script compares with cmp: go test ./cmd/relay -update.
var update = flag.Bool("update", false, "rewrite the txtar files compared with cmp from the actual output")

// TestMain lets the test binary act as the relay binary in the scripts,
// with the test-only fixture entity registered.
func TestMain(m *testing.M) {
	run := func() { os.Exit(relay(append(commands.Entities(), fixture.Entity))) }
	testscript.Main(m, map[string]func(){
		"relay": run,
	})
}

// TestScripts runs the functional tests: testdata/script/*.txtar at the repo
// root, each driving the real binary (docs/development.md).
func TestScripts(t *testing.T) {
	t.Parallel()
	testscript.Run(t, testscript.Params{
		Dir:                 filepath.Join("..", "..", "testdata", "script"),
		RequireExplicitExec: true,
		RequireUniqueNames:  true,
		UpdateScripts:       *update,
		Setup:               setup,
		Cmds:                map[string]func(ts *testscript.TestScript, neg bool, args []string){"stub": stub, "exits": exits},
	})
}

// sysTools are the host programs a script may run: what the stubs, the fakes
// and the scripts' own sh lines need. Every other program is absent from PATH
// unless a test stubs it, so no test depends on the tools of the host it runs
// on (tmux, claude, python3, git, ps, curl, sudo, uname).
var sysTools = []string{
	"sh", "bash", "env", "cat", "cp", "mv", "rm", "ln", "mkdir", "chmod", "ls", "grep", "sed", "sort",
	"tr", "head", "tail", "wc", "printf", "test", "dirname", "basename", "touch", "tar", "gzip",
	"mktemp", "sha256sum", "shasum", "perl", "cmp", "diff", "true", "false", "sleep",
}

// setup gives each script a PATH of its own: $WORK/bin first, for the stubs,
// then $WORK/sys, links to sysTools, then relay itself. It also clears what
// could reach outside the test (git's repo variables, the user's colours, the
// Claude Code config) and sets $REPO, the repository root, for the scripts
// that test install.sh and scripts/.
func setup(env *testscript.Env) error {
	bin := filepath.Join(env.WorkDir, "bin")
	sys := filepath.Join(env.WorkDir, "sys")
	for _, d := range []string{bin, sys} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	self, err := exec.LookPath("relay")
	if err != nil {
		return err
	}
	for _, name := range sysTools {
		path, err := exec.LookPath(name)
		if err != nil {
			continue // absent from this host: a script that needs it fails clearly
		}
		if err := os.Symlink(path, filepath.Join(sys, name)); err != nil {
			return err
		}
	}
	env.Setenv("PATH", strings.Join([]string{bin, sys, filepath.Dir(self)}, string(os.PathListSeparator)))
	env.Setenv("NO_COLOR", "1")
	env.Setenv("HOME", filepath.Join(env.WorkDir, "home"))
	env.Setenv("CLAUDE_CONFIG_DIR", "")
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		return err
	}
	env.Setenv("REPO", repo)
	// Removed, not emptied: git rejects an empty GIT_WORK_TREE.
	unset := []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_COMMON_DIR", "TMUX", "TMUX_PANE", "HANDOVER_RELAY", "HANDOVER_RELAY_OWN", "RELAY_GOOS", "RELAY_OS_RELEASE"}
	env.Vars = slices.DeleteFunc(env.Vars, func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		return slices.Contains(unset, name)
	})
	return nil
}

// stubScript is a stub tool: it appends its argv to $WORK/stubs.log, then
// prints stubs/<name>[.<call>].stdout and .stderr and exits with the number in
// stubs/<name>[.<call>].exit (0 without one); <call> counts from 1, so a test
// can answer each call differently.
const stubScript = `#!/bin/sh
name=%s
dir="$WORK/stubs"
calls="$dir/.$name.calls"
mkdir -p "$dir"
n=$(( $(cat "$calls" 2>/dev/null || echo 0) + 1 ))
echo "$n" >"$calls"
{
	printf '%%s' "$name"
	for a in "$@"; do printf ' %%s' "$a"; done
	echo
} >>"$WORK/stubs.log"
pick() {
	if [ -f "$dir/$name.$n.$1" ]; then cat "$dir/$name.$n.$1"; elif [ -f "$dir/$name.$1" ]; then cat "$dir/$name.$1"; fi
}
pick stdout
pick stderr >&2
code=$(pick exit)
exit "${code:-0}"
`

// stub installs a stub of each named tool in $WORK/bin: stub gh tmux.
func stub(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("unsupported: ! stub")
	}
	if len(args) == 0 {
		ts.Fatalf("usage: stub <tool>...")
	}
	for _, name := range args {
		if strings.ContainsAny(name, "/ '\"$") {
			ts.Fatalf("stub: bad tool name %q", name)
		}
		path := filepath.Join(ts.Getenv("WORK"), "bin", name)
		ts.Check(os.WriteFile(path, fmt.Appendf(nil, stubScript, name), 0o755))
	}
}

// exits runs a program and checks its exit status: exits 2 relay x y --bad.
// Its stdout and stderr are then checked as exec's.
func exits(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) < 2 {
		ts.Fatalf("usage: exits <status> <program> [args...]")
	}
	want, err := strconv.Atoi(args[0])
	ts.Check(err)
	got := 0
	if err := ts.Exec(args[1], args[2:]...); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			ts.Fatalf("%s: %v", args[1], err)
		}
		got = exit.ExitCode()
	}
	if got != want {
		ts.Fatalf("%s exited %d, want %d", args[1], got, want)
	}
}
