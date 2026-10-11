package session

import (
	"context"

	"github.com/FunDrivenDev/relay/internal/cli"
	"github.com/FunDrivenDev/relay/internal/fail"
	"github.com/FunDrivenDev/relay/internal/run"
)

type attachParams struct {
	Name string `json:"name"`
}

var attach = cli.New(cli.Spec{
	Verb:    "attach",
	Summary: "attaches to a session",
	Help: `Attaches to session <name>: tmux attach-session, or switch-client when
already under tmux. relay gives way to tmux, which then owns the terminal.`,
	Output:      "nothing: interactive",
	Params:      []cli.Param{{Name: "name", Help: "the session, as session list names it", Required: true}},
	Positional:  "name",
	Interactive: true,
}, func(ctx context.Context, env *cli.Env, p attachParams) (any, error) {
	if !exists(ctx, p.Name) {
		return nil, fail.New(fail.NotFound, "no session %s", p.Name)
	}
	if env.Getenv("TMUX") != "" {
		return nil, run.Exec("tmux", "switch-client", "-t", "="+p.Name)
	}
	return nil, run.Exec("tmux", "attach-session", "-t", "="+p.Name)
})
