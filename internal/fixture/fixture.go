// Package fixture declares the test-only `relay fixture` entity: commands
// that exercise every rule of the contract layer, run by the functional tests
// (testdata/script/contract_*.txtar). Only the test binary registers it.
package fixture

import (
	"context"
	"fmt"
	"strings"

	"github.com/FunDrivenDev/relay/internal/cli"
	"github.com/FunDrivenDev/relay/internal/fail"
	"github.com/FunDrivenDev/relay/internal/run"
	"github.com/FunDrivenDev/relay/internal/ui"
)

// Entity is the fixture entity.
var Entity = &cli.Entity{
	Name:    "fixture",
	Summary: "test-only commands that exercise the contract layer",
	Commands: []*cli.Command{
		echo,
		failCmd,
		pick,
		tool,
		raw,
		groupHello,
	},
}

type echoParams struct {
	Text  string   `json:"text"`
	Count int      `json:"count"`
	Mode  string   `json:"mode"`
	Tags  []string `json:"tags"`
	Loud  bool     `json:"loud"`
	Color string   `json:"color"`
	Args  []string `json:"args"`
}

type echoResult struct {
	Text   string   `json:"text"`
	Count  int      `json:"count"`
	Mode   string   `json:"mode"`
	Tags   []string `json:"tags"`
	Loud   bool     `json:"loud"`
	Color  string   `json:"color"`
	Args   []string `json:"args"`
	DryRun bool     `json:"dry_run"`
}

func (r echoResult) Human(t ui.Theme) string {
	text := r.Text
	if r.Mode == "upper" {
		text = strings.ToUpper(text)
	}
	if r.Loud {
		text += "!"
	}
	lines := make([]string, r.Count)
	for i := range lines {
		lines[i] = text
	}
	if r.DryRun {
		return t.Muted.Render("would print: ") + strings.Join(lines, " ")
	}
	return strings.Join(lines, "\n")
}

var echo = cli.New(cli.Spec{
	Verb:    "echo",
	Summary: "prints its input back",
	Help:    "Prints <text> --count times, and under --json every input it received.",
	Output:  `{"text", "count", "mode", "tags", "loud", "color", "args", "dry_run"}`,
	Params: []cli.Param{
		{Name: "text", Help: "the text to print", Required: true},
		{Name: "count", Short: "n", Placeholder: "n", Help: "how many times", Default: 1},
		{Name: "mode", Help: "the case of the text", Enum: []string{"plain", "upper"}, Default: "plain"},
		{Name: "tags", Placeholder: "tag,…", Help: "labels, echoed back"},
		{Name: "loud", Help: "ends the text with !"},
		{Name: "color", Placeholder: "when", Help: "a value given only with =", Optional: true, Implicit: "auto"},
		{Name: "args", Help: "arguments passed through"},
	},
	Positional:    "text",
	Rest:          "args",
	StateChanging: true,
}, func(_ context.Context, env *cli.Env, p echoParams) (any, error) {
	if p.Count < 1 {
		return nil, fail.Usagef("--count must be at least 1, not %d", p.Count)
	}
	if p.Tags == nil {
		p.Tags = []string{}
	}
	if p.Args == nil {
		p.Args = []string{}
	}
	env.Say("echoing %q", p.Text)
	return echoResult{p.Text, p.Count, p.Mode, p.Tags, p.Loud, p.Color, p.Args, env.DryRun}, nil
})

type failParams struct {
	Code   string `json:"code"`
	Result bool   `json:"result"`
	Quiet  bool   `json:"quiet"`
}

var failCmd = cli.New(cli.Spec{
	Verb:    "fail",
	Summary: "fails with the given error code",
	Help:    "Returns an error with <code>; --result prints a result first, --quiet only sets the exit status.",
	Output:  `with --result {"partial": true}`,
	Params: []cli.Param{
		{Name: "code", Help: "the error code", Required: true},
		{Name: "result", Help: "prints a result before failing"},
		{Name: "quiet", Help: "fails without a message"},
	},
	Positional: "code",
}, func(_ context.Context, _ *cli.Env, p failParams) (any, error) {
	var result any
	if p.Result {
		result = map[string]bool{"partial": true}
	}
	if p.Quiet {
		return result, fail.Quiet(fail.Code(p.Code))
	}
	return result, fail.New(fail.Code(p.Code), "failed with %s", p.Code)
})

type pickParams struct {
	Name string `json:"name"`
	Fast bool   `json:"fast"`
}

var pick = cli.New(cli.Spec{
	Verb:          "pick",
	Summary:       "an interactive command",
	Help:          "Asks a question on the terminal; refused under --json, but for --dry-run.",
	Output:        `with --dry-run {"would_pick"}`,
	Params:        []cli.Param{{Name: "name", Help: "what to pick", Required: true}, {Name: "fast", Help: "skips the question"}},
	Positional:    "name",
	Interactive:   true,
	DryRunJSON:    true,
	StateChanging: true,
}, func(_ context.Context, env *cli.Env, p pickParams) (any, error) {
	if env.DryRun {
		return map[string]string{"would_pick": p.Name}, nil
	}
	if !env.Terminal() {
		env.Say("no terminal: picking %s", p.Name)
	}
	return "picked " + p.Name, nil
})

type toolParams struct {
	Name string   `json:"name"`
	Args []string `json:"args"`
}

type toolResult struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

var tool = cli.New(cli.Spec{
	Verb:       "tool",
	Summary:    "runs an external tool",
	Help:       "Runs <name> with the arguments after -- and reports its output.",
	Output:     `{"stdout", "stderr"}`,
	Params:     []cli.Param{{Name: "name", Help: "the tool, looked up on PATH", Required: true}, {Name: "args", Help: "its arguments"}},
	Positional: "name",
	Rest:       "args",
}, func(ctx context.Context, _ *cli.Env, p toolParams) (any, error) {
	res, err := run.Tool(ctx, p.Name, p.Args...)
	if err != nil {
		return nil, err
	}
	return toolResult(res), nil
})

var raw = cli.New(cli.Spec{
	Verb:    "raw",
	Summary: "takes its arguments untouched",
	Help:    "Prints each argument on its own line, as a program whose caller fixes the interface.",
	Raw:     true,
}, func(_ context.Context, env *cli.Env, p cli.Raw) (any, error) {
	for _, a := range p.Args {
		if _, err := fmt.Fprintln(env.Stdout, a); err != nil {
			return nil, err
		}
	}
	return nil, nil
})

var groupHello = cli.New(cli.Spec{
	Verb:    "group hello",
	Summary: "a verb grouped under a noun",
	Help:    "Says hello: relay fixture group hello.",
	Output:  `"hello"`,
}, func(_ context.Context, _ *cli.Env, _ struct{}) (any, error) {
	return "hello", nil
})
