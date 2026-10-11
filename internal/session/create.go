package session

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/FunDrivenDev/relay/internal/cli"
	"github.com/FunDrivenDev/relay/internal/fail"
	"github.com/FunDrivenDev/relay/internal/run"
	"github.com/FunDrivenDev/relay/internal/ui"
)

type createParams struct {
	Name   string   `json:"name"`
	Dir    string   `json:"dir"`
	Prompt string   `json:"prompt"`
	Afk    bool     `json:"afk"`
	Until  string   `json:"until"`
	Yolo   bool     `json:"yolo"`
	Detach bool     `json:"detach"`
	Args   []string `json:"args"`
}

// Where the session runs: the values of the output's "where".
const (
	whereDetached = "detached" // a named session, not attached (--detach, --json)
	whereCurrent  = "current"  // under tmux with --name: the current session, renamed
	whereAttached = "attached" // a named session, attached on this terminal
	wherePane     = "pane"     // under tmux without --name: the current pane
	whereTab      = "tab"      // outside tmux without --name: a session tied to the tab
)

type createdSession struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Afk      bool   `json:"afk"`
	Where    string `json:"where"`
	Attached bool   `json:"attached"`
}

type createResult struct {
	Session createdSession `json:"session"`
}

// Human is empty: the message goes to stderr.
func (createResult) Human(ui.Theme) string { return "" }

type plannedSession struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Afk   bool   `json:"afk"`
	Where string `json:"where"`
}

type dryRunResult struct {
	DryRun    bool           `json:"dry_run"`
	Session   plannedSession `json:"session"`
	RelayArgs []string       `json:"relay_args"`
}

// Human is empty: the would-run line goes to stderr.
func (dryRunResult) Human(ui.Theme) string { return "" }

var create = cli.New(cli.Spec{
	Verb:    "create",
	Summary: "starts Claude Code under the relay, in a tmux session",
	Help: `Starts Claude Code under the handover relay (relay session run), in
<dir>, the current directory by default.

  no --name      outside tmux, a session tied to the tab, closed with it;
                 under tmux, in the current pane
  --name <name>  a named session that outlives the tab; under tmux, renames
                 the current session and starts Claude in the pane; refused
                 (code exists) when <name> exists
  --json         implies --detach

"where" says where Claude Code runs: detached, current (the current tmux
session, renamed), attached (a new named session on this terminal), pane or
tab.

relay_args, in the dry run's output, are the arguments relay session run
receives: --afk <goal> [--until <when>] [--yolo], then the claude options, then
the prompt when not an AFK run.`,
	Output: `{"session": {"name", "path", "afk", "where", "attached"}};
with --dry-run {"dry_run": true, "session": {"name", "path", "afk", "where"}, "relay_args"}`,
	Params: []cli.Param{
		{Name: "name", Help: "a named session that outlives the tab"},
		{Name: "dir", Help: "the directory Claude Code starts in (default: the current one)"},
		{Name: "prompt", Help: "the first message; the goal with --afk"},
		{Name: "afk", Help: "AFK run: unlimited relay until the goal is reached or blocked"},
		{Name: "until", Placeholder: "when", Help: "AFK run deadline: 8, tomorrow 8, mon 9:00, +10h"},
		{Name: "yolo", Help: "AFK run with no permission prompts, with guardrails and an audit log\n(/handover:stats, handover.py stats --audit <run>)"},
		{Name: "detach", Help: "creates the named session without attaching (implied by --json)"},
		{Name: "args", Placeholder: "claude options", Help: "the claude options"},
	},
	Positional:    "prompt",
	Rest:          "args",
	StateChanging: true,
}, runCreate)

func runCreate(ctx context.Context, env *cli.Env, p createParams) (any, error) {
	if env.JSON {
		p.Detach = true
	}
	switch {
	case p.Afk && p.Prompt == "":
		return nil, fail.Usagef("--afk requires a goal: the <prompt>")
	case !p.Afk && p.Until != "":
		return nil, fail.Usagef("--until requires --afk")
	case !p.Afk && p.Yolo:
		return nil, fail.Usagef("--yolo requires --afk")
	case p.Detach && p.Name == "":
		return nil, fail.Usagef("a detached session (--detach, --json) requires --name")
	}
	dir, err := resolveDir(p.Dir)
	if err != nil {
		return nil, err
	}
	if p.Name != "" && exists(ctx, p.Name) {
		return nil, fail.New(fail.Exists, "session %s already exists: relay session attach %s", p.Name, p.Name)
	}
	relayArgs := createRelayArgs(p)
	where := whereOf(p, env.Getenv("TMUX") != "")

	if env.DryRun {
		t := env.Theme
		env.Say("%s (%s) in %s: %s", t.Warn.Render("would run"), t.Arg.Render(where), t.Muted.Render(dir),
			t.Command.Render("relay session run"+quoteArgs(relayArgs)))
		planned := plannedSession{Name: p.Name, Path: dir, Afk: p.Afk, Where: where}
		return dryRunResult{DryRun: true, Session: planned, RelayArgs: relayArgs}, nil
	}
	// The relay needs the handover plugin: fail here, not in a tmux session
	// that would close at once.
	if _, err := handoverPy(env); err != nil {
		return nil, err
	}
	cmd, err := self()
	if err != nil {
		return nil, err
	}
	cmd = append(cmd, relayArgs...)

	switch where {
	case whereDetached:
		args := append([]string{"new-session", "-d", "-s", p.Name, "-c", dir}, cmd...)
		if _, err := run.Tool(ctx, "tmux", append(args, ";", "set-option", "destroy-unattached", "off")...); err != nil {
			return nil, err
		}
		t := env.Theme
		env.Say("%s %s created (%s)", t.OK.Render("session"), t.Title.Render(p.Name), t.Command.Render("relay session attach "+p.Name))
		return createResult{createdSession{Name: p.Name, Path: dir, Afk: p.Afk, Where: where}}, nil
	case whereAttached:
		args := append([]string{"new-session", "-s", p.Name, "-c", dir}, cmd...)
		return nil, run.Exec("tmux", append(args, ";", "set-option", "destroy-unattached", "off")...)
	case whereCurrent:
		// The relay ties the tmux session to the tab only when
		// HANDOVER_RELAY_OWN is set: a named session must outlive the tab.
		if _, err := run.Tool(ctx, "tmux", "rename-session", "--", p.Name,
			";", "set-option", "destroy-unattached", "off",
			";", "set-environment", "-u", "HANDOVER_RELAY_OWN"); err != nil {
			return nil, err
		}
		return nil, relayIn(ctx, env, dir, []string{"HANDOVER_RELAY_OWN="}, relayArgs)
	default:
		return nil, relayIn(ctx, env, dir, nil, relayArgs)
	}
}

// resolveDir makes dir absolute (the current directory when empty) and
// checks it is a directory.
func resolveDir(dir string) (string, error) {
	if dir == "" {
		// Getwd keeps $PWD when it names the current directory: the
		// shell's logical path.
		wd, err := os.Getwd()
		if err != nil {
			return "", fail.Wrap(fail.Failure, err, "cannot read the current directory: %v", err)
		}
		dir = wd
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fail.Wrap(fail.NotFound, err, "no such directory: %s", dir)
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		return "", fail.New(fail.NotFound, "no such directory: %s", dir)
	}
	return abs, nil
}

// createRelayArgs is the relay's argument list, in its canonical order:
// --afk <goal> [--until <when>] [--yolo], the claude options, then the prompt
// when not an AFK run (an AFK run's goal is its first prompt).
func createRelayArgs(p createParams) []string {
	out := []string{}
	if p.Afk {
		out = append(out, "--afk", p.Prompt)
		if p.Until != "" {
			out = append(out, "--until", p.Until)
		}
		if p.Yolo {
			out = append(out, "--yolo")
		}
	}
	out = append(out, p.Args...)
	if !p.Afk && p.Prompt != "" {
		out = append(out, p.Prompt)
	}
	return out
}

func whereOf(p createParams, inTmux bool) string {
	switch {
	case p.Detach:
		return whereDetached
	case p.Name != "" && inTmux:
		return whereCurrent
	case p.Name != "":
		return whereAttached
	case inTmux:
		return wherePane
	default:
		return whereTab
	}
}

// relayIn runs the relay in dir, in this process.
func relayIn(ctx context.Context, env *cli.Env, dir string, extra, args []string) error {
	if err := os.Chdir(dir); err != nil {
		return fail.Wrap(fail.NotFound, err, "no such directory: %s", dir)
	}
	if err := os.Setenv("PWD", dir); err != nil {
		return fail.Wrap(fail.Failure, err, "cannot set PWD: %v", err)
	}
	return relay(ctx, env, extra, args)
}

// quoteArgs writes args as shell words, each after a space.
func quoteArgs(args []string) string {
	var b strings.Builder
	for _, a := range args {
		b.WriteString(" " + shellQuote(a))
	}
	return b.String()
}

func shellQuote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && !strings.ContainsRune("-_./=:,@%+", r)
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
