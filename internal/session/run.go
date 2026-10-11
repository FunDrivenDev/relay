package session

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/FunDrivenDev/relay/internal/cli"
	"github.com/FunDrivenDev/relay/internal/fail"
	"github.com/FunDrivenDev/relay/internal/run"
)

// runCmd is the relay itself (formerly the handover plugin's claude-relay
// script). Its callers fix its interface (session create, its own re-exec
// inside tmux), and its arguments mix the relay's options with claude's: it
// is raw, outside the JSON contract.
var runCmd = cli.New(cli.Spec{
	Verb:    "run",
	Summary: "runs Claude Code with the handover relay on",
	Help: `Runs Claude Code with the handover relay on: once a handover is ready and
the session has written its closing reply, a fresh session resumes from it
without the user, up to the plugin's relay_limit handovers.

  relay session run [<claude args>…]
  relay session run --afk <goal> [--until <when>] [--yolo] [<claude args>…]

--afk, --until and --yolo may come anywhere; every other argument goes to
claude untouched.

--afk runs toward <goal> while the user is away: no relay_limit; sessions are
sent back to work until one ends a reply on "AFK done: <why>" or "AFK blocked:
<why>". The goal is also the first prompt. --until (8, 8:30, tomorrow 8,
mon 9:00, +10h, or @<epoch>) is a deadline past which no new handover starts,
resolved once, here, by handover.py until. --yolo skips permission prompts
(--dangerously-skip-permissions), with the deny rules of handover.py
yolo-settings merged into the user's own --settings; the plugin's hooks write
every tool call to an audit log (/handover:stats). --until and --yolo require
--afk (exit 2).

Outside tmux, with tmux installed, it starts again inside a new tmux session:
an AFK run in a named session afk-<directory>[-<n>] that outlives the tab,
otherwise a session tied to the tab. Inside tmux it gives way to claude, and
the relay types /clear and the resume message into the pane. Without tmux it
loops: the relay leaves the resume message in $HANDOVER_RELAY_FILE and ends
claude, and the loop starts claude again with the same arguments plus that
message.

The environment it sets is the contract with handover.py:
docs/handover-relay.md. handover.py is found through the handover plugin's
installPath in ${CLAUDE_CONFIG_DIR:-~/.claude}/plugins/installed_plugins.json
(code relay_missing without it).`,
	Raw: true,
}, func(ctx context.Context, env *cli.Env, p cli.Raw) (any, error) {
	return nil, relay(ctx, env, nil, p.Args)
})

// relayOpts are the relay's own options, read from its arguments.
type relayOpts struct {
	goal  string   // --afk <goal>: an AFK run
	until string   // --until <when>, or @<epoch> once resolved
	yolo  bool     // --yolo
	args  []string // everything else, for claude
}

// afk is the relay's own options as arguments: what a restart passes on.
func (o relayOpts) afk() []string {
	var out []string
	if o.goal != "" {
		out = append(out, "--afk", o.goal)
	}
	if o.until != "" {
		out = append(out, "--until", o.until)
	}
	if o.yolo {
		out = append(out, "--yolo")
	}
	return out
}

// parseRelay reads the relay's options out of argv, in any position; the
// rest goes to claude in its order. A value is the next argument, whatever
// it starts with.
func parseRelay(argv []string) (relayOpts, error) {
	var o relayOpts
	value := func(i int, what string) (string, error) {
		if i+1 >= len(argv) || argv[i+1] == "" {
			return "", fail.Usagef("%s needs %s", argv[i], what)
		}
		return argv[i+1], nil
	}
	for i := 0; i < len(argv); i++ {
		var err error
		switch argv[i] {
		case "--afk":
			o.goal, err = value(i, "a goal")
			i++
		case "--until":
			o.until, err = value(i, "a deadline")
			i++
		case "--yolo":
			o.yolo = true
		default:
			o.args = append(o.args, argv[i])
		}
		if err != nil {
			return o, err
		}
	}
	switch {
	case o.until != "" && o.goal == "":
		return o, fail.Usagef("--until goes with --afk")
	case o.yolo && o.goal == "":
		return o, fail.Usagef("--yolo goes with --afk")
	}
	return o, nil
}

// afkSessionName is the tmux session of an AFK run started in dir:
// afk-<basename>, with . and : (tmux separators) as -, then -2, -3… while
// taken.
func afkSessionName(dir string, taken func(string) bool) string {
	base := "afk-" + strings.NewReplacer(".", "-", ":", "-").Replace(filepath.Base(dir))
	name := base
	for n := 2; taken(name); n++ {
		name = base + "-" + strconv.Itoa(n)
	}
	return name
}

// splitSettings takes the user's --settings out of claude's arguments: claude
// takes a single --settings, so a YOLO run merges its deny rules into it.
func splitSettings(args []string) (settings string, rest []string, err error) {
	for len(args) > 0 {
		a := args[0]
		args = args[1:]
		switch {
		case a == "--settings":
			if len(args) == 0 || args[0] == "" {
				return "", nil, fail.Usagef("--settings needs a value")
			}
			settings, args = args[0], args[1:]
		case strings.HasPrefix(a, "--settings="):
			settings = strings.TrimPrefix(a, "--settings=")
		default:
			rest = append(rest, a)
		}
	}
	return settings, rest, nil
}

// relay is relay session run: extra are KEY=value pairs added to the
// environment of every program it starts (session create passes
// HANDOVER_RELAY_OWN= for a named session).
func relay(ctx context.Context, env *cli.Env, extra, argv []string) error {
	o, err := parseRelay(argv)
	if err != nil {
		return err
	}
	if o.until != "" && !strings.HasPrefix(o.until, "@") {
		// Resolved once, here: 8 means the next 8:00 from now, not from a
		// later restart.
		epoch, err := handover(ctx, env, extra, "until", o.until)
		if err != nil {
			return err
		}
		o.until = "@" + epoch
	}
	extra = append(slices.Clip(extra), "HANDOVER_RELAY=1")
	getenv := func(key string) string {
		for i := len(extra) - 1; i >= 0; i-- {
			if k, v, _ := strings.Cut(extra[i], "="); k == key {
				return v
			}
		}
		return env.Getenv(key)
	}
	opts := run.Opts{Env: extra}

	if getenv("TMUX") == "" && run.Has("tmux") {
		return intoTmux(ctx, env, opts, o)
	}

	args := o.args
	if o.goal != "" {
		opts.Env = append(opts.Env, "HANDOVER_AFK_GOAL="+o.goal)
		if o.until != "" {
			opts.Env = append(opts.Env, "HANDOVER_AFK_UNTIL="+strings.TrimPrefix(o.until, "@"))
		}
		if o.yolo {
			opts.Env = append(opts.Env, "HANDOVER_AFK_YOLO=1")
			user, rest, err := splitSettings(args)
			if err != nil {
				return err
			}
			hargs := []string{"yolo-settings"}
			if user != "" {
				hargs = append(hargs, "--merge", user)
			}
			settings, err := handover(ctx, env, opts.Env, hargs...)
			if err != nil {
				return err
			}
			args = append([]string{"--dangerously-skip-permissions", "--settings", settings}, rest...)
		}
	}

	if getenv("TMUX") != "" {
		// A tmux session started by the relay goes with its tab, as claude
		// would without tmux; an AFK run stays when the tab closes.
		pane := getenv("TMUX_PANE")
		switch {
		case o.goal != "":
			if _, err := run.Tool(ctx, "tmux", "set-option", "-t", pane, "destroy-unattached", "off"); err != nil {
				return err
			}
		case getenv("HANDOVER_RELAY_OWN") != "":
			if _, err := run.Tool(ctx, "tmux", "set-option", "-t", pane, "destroy-unattached", "on"); err != nil {
				return err
			}
		}
		if o.goal != "" {
			args = append(args, o.goal)
		}
		return opts.Exec("claude", args...)
	}
	return loop(ctx, opts, args, o.goal)
}

// intoTmux starts the relay again inside a new tmux session, in the current
// directory: an AFK run in a named session that outlives the tab, any other
// in a session tied to the tab (HANDOVER_RELAY_OWN=1).
func intoTmux(ctx context.Context, env *cli.Env, opts run.Opts, o relayOpts) error {
	dir, err := os.Getwd()
	if err != nil {
		return fail.Wrap(fail.Failure, err, "cannot read the current directory: %v", err)
	}
	cmd, err := self()
	if err != nil {
		return err
	}
	if o.goal != "" {
		name := afkSessionName(dir, func(n string) bool { return exists(ctx, n) })
		t := env.Theme
		env.Say("%s %s (%s)", t.Warn.Render("AFK run in tmux session"), t.Title.Render(name), t.Command.Render("tmux attach -t "+name))
		args := append([]string{"new-session", "-s", name, "-c", dir}, cmd...)
		args = append(args, o.afk()...)
		return opts.Exec("tmux", append(args, o.args...)...)
	}
	args := append([]string{"new-session", "-c", dir, "-e", "HANDOVER_RELAY_OWN=1"}, cmd...)
	return opts.Exec("tmux", append(args, o.args...)...)
}

// loop runs claude until a session ends without a relay: the relay writes the
// resume message to $HANDOVER_RELAY_FILE and ends claude, and the loop starts
// it again with the same arguments plus that message. The goal of an AFK run
// is the first prompt only.
func loop(ctx context.Context, opts run.Opts, args []string, goal string) error {
	f, err := os.CreateTemp("", "claude-relay.")
	if err != nil {
		return fail.Wrap(fail.Failure, err, "cannot create the relay file: %v", err)
	}
	file := f.Name()
	_ = f.Close()
	defer func() { _ = os.Remove(file) }()
	opts.Env = append(opts.Env, "HANDOVER_RELAY_FILE="+file, "HANDOVER_RELAY_PID="+strconv.Itoa(os.Getpid()))

	var message []string
	if goal != "" {
		message = []string{goal}
	}
	// Ctrl-C belongs to claude: it reaches this process too, and must not
	// end claude or the loop.
	ctx = context.WithoutCancel(ctx)
	for {
		err := opts.Attach(ctx, "claude", append(slices.Clip(args), message...)...)
		if fail.CodeOf(err) == fail.ToolMissing {
			return err
		}
		_, _ = run.Opts{Stdin: os.Stdin}.Tool(ctx, "stty", "sane")
		data, err := os.ReadFile(file) //nolint:gosec // the file this loop created
		if err != nil || len(data) == 0 {
			return nil // no message, or no file: the chain ends
		}
		message = []string{strings.TrimRight(string(data), "\n")}
		if err := os.WriteFile(file, nil, 0o600); err != nil {
			return fail.Wrap(fail.Failure, err, "cannot empty the relay file: %v", err)
		}
	}
}

// handover runs python3 handover.py with args and returns its stdout without
// its trailing newlines; what it says for people (stderr) is passed on.
func handover(ctx context.Context, env *cli.Env, extra []string, args ...string) (string, error) {
	py, err := handoverPy(env)
	if err != nil {
		return "", err
	}
	res, err := run.Opts{Env: extra}.Tool(ctx, "python3", append([]string{py}, args...)...)
	if err != nil {
		return "", err // its message carries the first line of stderr
	}
	if res.Stderr != "" {
		_, _ = io.WriteString(env.Stderr, res.Stderr)
	}
	return strings.TrimRight(res.Stdout, "\n"), nil
}
