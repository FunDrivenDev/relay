// Package notifier holds the `relay notifier` entity: how the handover
// plugin's alerts reach you here, as `handover.py notifier` reports it
// (docs/notifier.md). relay reads the plugin's report; it never detects the
// environment itself.
package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FunDrivenDev/relay/internal/cli"
	"github.com/FunDrivenDev/relay/internal/fail"
	"github.com/FunDrivenDev/relay/internal/run"
	"github.com/FunDrivenDev/relay/internal/session"
	"github.com/FunDrivenDev/relay/internal/ui"
)

// Guide is the handover plugin's notifications guide.
const Guide = "https://github.com/FunDrivenDev/claude-plugins/blob/main/plugins/handover/docs/notifications.md"

// Entity is the notifier entity.
var Entity = &cli.Entity{
	Name:    "notifier",
	Summary: "how the handover plugin's alerts reach you, checked and tested",
	Commands: []*cli.Command{
		check,
		test,
	},
}

// Environment is where the plugin runs, as it detects it.
type Environment struct {
	// OS is macos, linux, wsl or other.
	OS string `json:"os"`
	// Desktop: a desktop notification can reach a screen here.
	Desktop bool            `json:"desktop"`
	SSH     bool            `json:"ssh"`
	Why     string          `json:"why"`
	Tools   map[string]bool `json:"tools"`
}

// Report is `handover.py notifier`'s report.
type Report struct {
	Environment      Environment `json:"environment"`
	Notifier         string      `json:"notifier"`
	NotifyCommandSet bool        `json:"notify_command_set"`
	// Adapters are the notifiers an alert goes through, the bell last.
	Adapters []string `json:"adapters"`
	// ReachesYou is false when only the bell rings.
	ReachesYou bool     `json:"reaches_you"`
	Hints      []string `json:"hints"`
}

// Advice is the plugin's hints, with the guide's URL when they lack it.
func (r Report) Advice() []string {
	advice := append([]string{}, r.Hints...)
	if !strings.Contains(strings.Join(advice, "\n"), Guide) {
		advice = append(advice, "guide: "+Guide)
	}
	return advice
}

// Where describes the environment in a few words: "linux, desktop (a Linux
// desktop session)", "linux, headless, over SSH (…)".
func (e Environment) Where() string {
	kind := "headless"
	if e.Desktop {
		kind = "desktop"
	}
	if e.SSH {
		kind += ", over SSH"
	}
	return fmt.Sprintf("%s, %s (%s)", e.OS, kind, e.Why)
}

// Human prints the report: where, through what, whether it reaches you, and
// the advice when it may not.
func (r Report) Human(t ui.Theme) string {
	var b strings.Builder
	command := "notify_command not set"
	if r.NotifyCommandSet {
		command = "notify_command set"
	}
	fmt.Fprintf(&b, "%s  %s\n", t.Title.Render("environment"), r.Environment.Where())
	fmt.Fprintf(&b, "%s     %s (%s)\n", t.Title.Render("notifier"), r.Notifier, command)
	fmt.Fprintf(&b, "%s     %s\n", t.Title.Render("adapters"), strings.Join(r.Adapters, ", "))
	if r.ReachesYou {
		fmt.Fprintf(&b, "%s alerts reach you\n", t.OK.Render("✓"))
	} else {
		fmt.Fprintf(&b, "%s only the terminal bell rings: alerts may not reach you\n", t.Warn.Render("!"))
	}
	if len(r.Hints) > 0 || !r.ReachesYou {
		for _, a := range r.Advice() {
			fmt.Fprintf(&b, "  %s\n", t.Command.Render(a))
		}
	}
	return b.String()
}

// Inspect runs `python3 handover.py notifier`, with --test when send is set,
// and reads its report. It fails with relay_missing without the plugin,
// tool_missing without python3.
func Inspect(ctx context.Context, env *cli.Env, send bool) (Report, *bool, error) {
	plugin, err := session.FindPlugin(env)
	if err != nil {
		return Report{}, nil, err
	}
	args := []string{plugin.Script, "notifier"}
	if send {
		args = append(args, "--test")
	}
	// Its stderr repeats the hints, which the report carries.
	res, err := run.Tool(ctx, "python3", args...)
	if err != nil {
		return Report{}, nil, err
	}
	var out struct {
		Report
		Sent *bool `json:"sent"`
	}
	if err := json.Unmarshal([]byte(res.Stdout), &out); err != nil {
		return Report{}, nil, fail.Wrap(fail.ToolFailed, err, "handover.py notifier: unreadable report (handover 0.16.0 or later is needed): %v", err)
	}
	if out.Adapters == nil {
		out.Adapters = []string{}
	}
	if out.Hints == nil {
		out.Hints = []string{}
	}
	return out.Report, out.Sent, nil
}

type checkParams struct{}

var check = cli.New(cli.Spec{
	Verb:    "check",
	Summary: "reports how the handover plugin's alerts reach you here",
	Help: `Runs the handover plugin's handover.py notifier and prints its report: the
environment it detects (macos, linux, wsl, other; a desktop or headless, and
why; over SSH), the notifier option, the adapters an alert goes through (the
terminal bell last), whether they reach you, and the plugin's hints.

Claude Code sees the environment of its tmux session: run this where the
sessions start (a desktop terminal, or the SSH session).

Read-only: exit 0 whatever it reports. The plugin's options and the Slack or
ntfy setups: ` + Guide,
	Output: `{"environment": {"os", "desktop", "ssh", "why", "tools"}, "notifier", "notify_command_set", "adapters", "reaches_you", "hints"}`,
}, func(ctx context.Context, env *cli.Env, _ checkParams) (any, error) {
	r, _, err := Inspect(ctx, env, false)
	if err != nil {
		return nil, err
	}
	return r, nil
})

// TestResult is what notifier test prints.
type TestResult struct {
	DryRun bool `json:"dry_run"`
	// Sent: the alert got through one notifier at least, the bell included.
	Sent bool `json:"sent"`
	Report
}

// Human prints whether the alert went out, then the report.
func (r TestResult) Human(t ui.Theme) string {
	var head string
	switch {
	case r.DryRun:
		head = t.Warn.Render("would send") + " a test alert through " + strings.Join(r.Adapters, ", ")
	case r.Sent && r.ReachesYou:
		head = t.OK.Render("sent") + " a test alert through " + strings.Join(r.Adapters, ", ")
	case r.Sent:
		head = t.Warn.Render("sent") + " a test alert: only the terminal bell rang"
	default:
		head = t.Error.Render("not sent") + ": no notifier got through, not even the bell"
	}
	return head + "\n\n" + r.Report.Human(t)
}

type testParams struct{}

var test = cli.New(cli.Spec{
	Verb:    "test",
	Summary: "sends a test alert through the handover plugin's notifier",
	Help: `Runs handover.py notifier --test: sends "Test notification: alerts reach
you here." through every adapter notifier check lists, the terminal bell
included, then prints the report with sent.

--dry-run sends nothing and reports where it would go. Exit 1 with code
not_sent when no adapter got through, not even the bell; a bell alone counts
as sent (reaches_you says whether more than the bell rang).`,
	Output:        `{"dry_run", "sent", "environment": {…}, "notifier", "notify_command_set", "adapters", "reaches_you", "hints"}: notifier check's report, with dry_run and sent`,
	StateChanging: true,
}, func(ctx context.Context, env *cli.Env, _ testParams) (any, error) {
	r, sent, err := Inspect(ctx, env, !env.DryRun)
	if err != nil {
		return nil, err
	}
	res := TestResult{DryRun: env.DryRun, Report: r}
	if env.DryRun {
		return res, nil
	}
	res.Sent = sent != nil && *sent
	if !res.Sent {
		return res, &fail.Error{
			Code: fail.NotSent, Silent: !env.JSON,
			Message: "the test alert got through no notifier, not even the bell (" + Guide + ")",
		}
	}
	return res, nil
})
