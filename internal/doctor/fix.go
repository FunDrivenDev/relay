package doctor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"charm.land/huh/v2"

	"github.com/FunDrivenDev/relay/internal/cli"
	"github.com/FunDrivenDev/relay/internal/fail"
	"github.com/FunDrivenDev/relay/internal/run"
	"github.com/FunDrivenDev/relay/internal/ui"
)

type fixParams struct {
	Yes bool `json:"yes"`
}

// StepResult is one step of a fix, run or planned.
type StepResult struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	Ran     bool   `json:"ran"`
	OK      bool   `json:"ok"`
	Error   string `json:"error"`
}

// FixResult is what doctor fix prints.
type FixResult struct {
	OK     bool         `json:"ok"`
	DryRun bool         `json:"dry_run"`
	Steps  []StepResult `json:"steps"`
	// Checks are the checks after the fix (before it, on a dry run).
	Checks []Check `json:"checks"`
}

// Human prints the steps, then the checklist they leave.
func (r FixResult) Human(t ui.Theme) string {
	var b strings.Builder
	for _, s := range r.Steps {
		mark := t.Muted.Render("·")
		switch {
		case s.Ran && s.OK:
			mark = t.OK.Render("✓")
		case s.Ran:
			mark = t.Error.Render("✗")
		}
		fmt.Fprintf(&b, "%s %s\n", mark, t.Command.Render(s.Command))
	}
	if len(r.Steps) > 0 {
		b.WriteString("\n")
	}
	b.WriteString(checklist(t, r.Checks))
	if r.OK && !r.DryRun {
		fmt.Fprintf(&b, "\n%s\n", t.OK.Render("relay is ready."))
	}
	return b.String()
}

var fix = cli.New(cli.Spec{
	Verb:    "fix",
	Summary: "installs what doctor check finds missing",
	Help: `Runs the fixes doctor check plans, in order, each shown before it runs:

  1. sudo apt-get update, then
     sudo apt-get install -y <the missing packages>  (tmux, python3, procps,
     git, libnotify-bin; curl when Claude Code's installer needs it)
  2. curl -fsSL https://claude.ai/install.sh | bash   (Claude Code)
  3. claude plugin marketplace add FunDrivenDev/claude-plugins  (when unknown)
     claude plugin install handover@fundrivendev

It asks first on a terminal; --yes skips the question, and is required under
--json. The first step that fails stops the fix (code fix_failed). The checks
run again at the end: a required one still failing ends on
requirements_missing (a tmux or Python older than the floor has no automatic
fix: upgrade the distribution or the package).`,
	Output: `{"ok", "dry_run", "steps": [{"name", "command", "ran", "ok", "error"}], "checks": [doctor check's checks]}`,
	Params: []cli.Param{
		{Name: "yes", Short: "y", Help: "runs the fixes without asking"},
	},
	StateChanging: true,
}, func(ctx context.Context, env *cli.Env, p fixParams) (any, error) {
	if env.JSON && !p.Yes && !env.DryRun {
		return nil, fail.Usagef("relay doctor fix --json needs --yes (or --dry-run): it cannot ask")
	}
	before := inspect(ctx, env)
	res := FixResult{OK: before.OK, DryRun: env.DryRun, Steps: []StepResult{}, Checks: before.Checks}
	for _, s := range before.Fixes {
		res.Steps = append(res.Steps, StepResult{Name: s.Name, Command: s.Command})
	}
	t := env.Theme
	if len(res.Steps) == 0 {
		if before.OK {
			env.Say("%s", t.OK.Render("nothing to fix"))
		}
		return res, before.err(env)
	}
	if env.DryRun {
		for _, s := range res.Steps {
			env.Say("%s %s", t.Warn.Render("would run"), t.Command.Render(s.Command))
		}
		return res, nil
	}
	if !p.Yes {
		if err := confirm(env, res.Steps); err != nil {
			return nil, err
		}
	}
	// addLocalBin widens relay's PATH for the steps only: the checks after
	// them see the shell's, where tmux will look for claude.
	path := os.Getenv("PATH")
	for i := range res.Steps {
		s := &res.Steps[i]
		env.Say("%s %s", t.Title.Render("running"), t.Command.Render(s.Command))
		s.Ran = true
		if err := runStep(ctx, env, s.Command); err != nil {
			s.Error = err.Error()
			restorePath(path)
			res.OK, res.Checks = false, inspect(ctx, env).Checks
			return res, &fail.Error{
				Code: fail.FixFailed, Silent: !env.JSON, Err: err,
				Message: fmt.Sprintf("%s failed: %s", s.Command, err),
			}
		}
		s.OK = true
		if s.Name == "claude" {
			addLocalBin(env)
		}
	}
	restorePath(path)
	after := inspect(ctx, env)
	res.OK, res.Checks = after.OK, after.Checks
	return res, after.err(env)
})

// restorePath puts back the PATH relay started with.
func restorePath(path string) {
	_ = os.Setenv("PATH", path)
}

// confirm shows the plan and asks before running it: on a terminal only.
func confirm(env *cli.Env, steps []StepResult) error {
	if !env.Terminal() {
		return fail.New(fail.TTYRequired, "relay doctor fix asks before installing: run it on a terminal, or with --yes")
	}
	t := env.Theme
	env.Say("%s", t.Title.Render("relay doctor fix will run:"))
	for _, s := range steps {
		env.Say("  %s", t.Command.Render(s.Command))
	}
	ok := false
	err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Title("Run these commands?").Value(&ok),
	)).WithTheme(ui.FormTheme).WithInput(env.Stdin).WithOutput(env.Stderr).Run()
	switch {
	case errors.Is(err, huh.ErrUserAborted):
		return fail.Quiet(fail.Interrupted)
	case err != nil:
		return fail.Wrap(fail.Failure, err, "confirm: %s", err)
	case !ok:
		return fail.Quiet(fail.Declined)
	}
	return nil
}

// runStep runs one fix command with sh: on stderr for people, captured under
// --json, where stderr carries only the error envelope.
func runStep(ctx context.Context, env *cli.Env, command string) error {
	if env.JSON {
		_, err := run.Opts{Stdin: env.Stdin}.Tool(ctx, "sh", "-c", command)
		return err
	}
	return run.Opts{Stdin: env.Stdin, Stdout: env.Stderr, Stderr: env.Stderr}.Attach(ctx, "sh", "-c", command)
}

// addLocalBin puts ~/.local/bin, where Claude Code's installer puts claude,
// on relay's PATH for the steps and checks that follow, and says to add it to
// the shell's when it is missing there.
func addLocalBin(env *cli.Env) {
	home := env.Getenv("HOME")
	if home == "" {
		return
	}
	bin := filepath.Join(home, ".local", "bin")
	path := os.Getenv("PATH")
	if slices.Contains(filepath.SplitList(path), bin) {
		return
	}
	_ = os.Setenv("PATH", bin+string(os.PathListSeparator)+path)
	env.Say("%s %s is not on your PATH: add it to your shell's profile (%s)",
		env.Theme.Warn.Render("note"), bin, env.Theme.Command.Render(`export PATH="$HOME/.local/bin:$PATH"`))
}
