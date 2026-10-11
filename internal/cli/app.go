package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"golang.org/x/term"

	"github.com/FunDrivenDev/relay/internal/fail"
	"github.com/FunDrivenDev/relay/internal/ui"
)

// App is the relay program: its entities, dispatched by noun then verb.
type App struct {
	Name     string // "relay"
	Version  string
	Entities []*Entity
}

// NewApp returns the app serving entities.
func NewApp(name, version string, entities []*Entity) *App {
	for _, e := range entities {
		for _, c := range e.Commands {
			c.entity = e.Name
			c.app = name
		}
	}
	return &App{Name: name, Version: version, Entities: entities}
}

// System is what the program reads and writes; os's own in main, buffers in
// the unit tests.
type System struct {
	Stdin   io.Reader
	Stdout  io.Writer // data only
	Stderr  io.Writer // human messages and errors
	Environ []string
}

// OS is the process's own System.
func OS() System {
	return System{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Environ: os.Environ()}
}

// Env is what a command's run function gets besides its params.
type Env struct {
	Stdin  io.Reader
	Stdout io.Writer // data only
	Stderr io.Writer // human messages
	// Command is the full command name: relay <entity> <verb>.
	Command string
	// JSON is set under --json: print nothing for people.
	JSON bool
	// DryRun is set by --dry-run (state-changing commands only).
	DryRun  bool
	Environ []string
	// Theme styles the messages for people (stderr): Rich when stderr is a
	// colour terminal.
	Theme ui.Theme
}

// Say prints a message for people on stderr; nothing under --json, where
// stderr only carries the error envelope.
func (e *Env) Say(format string, args ...any) {
	if e.JSON {
		return
	}
	fmt.Fprintf(ui.Writer(e.Stderr, e.Environ), format+"\n", args...)
}

// Getenv reads a variable of the command's environment.
func (e *Env) Getenv(key string) string {
	for _, kv := range slices.Backward(e.Environ) {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			return v
		}
	}
	return ""
}

// Terminal reports whether stdin and stdout are both a terminal, as an
// interactive prompt needs.
func (e *Env) Terminal() bool {
	return isTerminal(e.Stdin) && isTerminal(e.Stdout)
}

func isTerminal(f any) bool {
	file, ok := f.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

// Humanizer is a result with a human form; any other result prints as
// indented JSON, in both modes.
type Humanizer interface {
	Human(t ui.Theme) string
}

// Main runs the command args name and returns the exit status.
func (a *App) Main(ctx context.Context, args []string, sys System) int {
	jsonOut := wantsJSON(args)
	err := a.dispatch(ctx, args, sys)
	if err != nil && !fail.IsSilent(err) {
		a.report(sys, err, jsonOut)
	}
	return fail.ExitStatus(err)
}

func (a *App) dispatch(ctx context.Context, args []string, sys System) error {
	if len(args) == 0 || isHelp(args) {
		return a.usage(sys, wantsJSON(args), nil, "")
	}
	head := args[0]
	if head == "--version" {
		return a.version(sys, args[1:])
	}
	ent := a.entity(head)
	if ent == nil {
		return fail.Usagef("unknown command: %s (%s --help)", head, a.Name)
	}
	if len(args) == 1 || isHelp(args[1:]) {
		return a.usage(sys, wantsJSON(args), ent, "")
	}
	if cmd := ent.command(args[1]); cmd != nil {
		return a.runCommand(ctx, cmd, args[2:], sys)
	}
	if !ent.isGroup(args[1]) {
		return fail.Usagef("unknown verb: %s %s (%s %s --help)", ent.Name, args[1], a.Name, ent.Name)
	}
	group := args[1]
	if len(args) == 2 || isHelp(args[2:]) {
		return a.usage(sys, wantsJSON(args), ent, group)
	}
	cmd := ent.command(group + " " + args[2])
	if cmd == nil {
		return fail.Usagef("unknown verb: %s %s %s (%s %s %s --help)", ent.Name, group, args[2], a.Name, ent.Name, group)
	}
	return a.runCommand(ctx, cmd, args[3:], sys)
}

// isGroup reports whether name groups verbs of the entity (name <verb>).
func (e *Entity) isGroup(name string) bool {
	for _, c := range e.Commands {
		if strings.HasPrefix(c.Verb, name+" ") {
			return true
		}
	}
	return false
}

// isHelp reports whether args only ask for help: help, -h, --help, with or
// without --json.
func isHelp(args []string) bool {
	help := false
	for _, a := range args {
		switch a {
		case "help", "-h", "--help":
			help = true
		case "--json":
		default:
			return false
		}
	}
	return help
}

func (a *App) entity(name string) *Entity {
	for _, e := range a.Entities {
		if e.Name == name {
			return e
		}
	}
	return nil
}

func (e *Entity) command(verb string) *Command {
	for _, c := range e.Commands {
		if c.Verb == verb {
			return c
		}
	}
	return nil
}

// Commands lists every command of the app, with their entity set.
func (a *App) Commands() []*Command {
	var out []*Command
	for _, e := range a.Entities {
		out = append(out, e.Commands...)
	}
	return out
}

func (a *App) runCommand(ctx context.Context, cmd *Command, args []string, sys System) error {
	if len(cmd.problems) > 0 {
		return fail.New(fail.Failure, "internal: %s is misdeclared: %s", cmd.Name(), strings.Join(cmd.problems, "; "))
	}
	inv, err := cmd.parse(args, sys.Stdin)
	if err != nil {
		return withHelpHint(err, cmd.Name())
	}
	if inv.help {
		if inv.json {
			return writeJSON(sys.Stdout, cmd.InputSpec())
		}
		_, err := io.WriteString(ui.Writer(sys.Stdout, sys.Environ), cmd.help(ui.Frappe))
		return err
	}
	// An interactive command whose dry run asks nothing runs it under --json.
	if cmd.Interactive && inv.json && (!cmd.DryRunJSON || !inv.values.dryRun()) {
		return fail.New(fail.TTYRequired, "%s is interactive: run it in a terminal, without --json: %s",
			cmd.Name(), cmd.FlagForm(inv.values))
	}
	env := &Env{
		Stdin: sys.Stdin, Stdout: sys.Stdout, Stderr: sys.Stderr,
		Command: cmd.Name(), JSON: inv.json, DryRun: inv.values.dryRun(),
		Environ: sys.Environ, Theme: ui.For(sys.Stderr, sys.Environ),
	}
	result, err := cmd.run(ctx, env, cmd.bind(inv.values))
	if result != nil {
		if werr := writeResult(sys, result, inv.json); werr != nil && err == nil {
			err = werr
		}
	}
	if err != nil && ctx.Err() != nil && fail.CodeOf(err) == fail.Failure {
		err = fail.Wrap(fail.Interrupted, err, "%s interrupted", cmd.Name())
	}
	if fail.CodeOf(err) == fail.Usage {
		err = withHelpHint(err, cmd.Name())
	}
	return err
}

// withHelpHint ends a usage error's message with the help command to read.
func withHelpHint(err error, name string) error {
	var e *fail.Error
	if errors.As(err, &e) && e.Code == fail.Usage && !strings.HasSuffix(e.Message, "--help)") {
		return &fail.Error{Code: e.Code, Message: fmt.Sprintf("%s (%s --help)", e.Message, name), Err: e.Err}
	}
	return err
}

func writeResult(sys System, result any, jsonOut bool) error {
	if !jsonOut {
		switch r := result.(type) {
		case Humanizer:
			text := r.Human(ui.For(sys.Stdout, sys.Environ))
			if text != "" && !strings.HasSuffix(text, "\n") {
				text += "\n"
			}
			_, err := io.WriteString(ui.Writer(sys.Stdout, sys.Environ), text)
			return err
		case string:
			_, err := fmt.Fprintln(sys.Stdout, r)
			return err
		}
	}
	return writeJSON(sys.Stdout, result)
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// report prints err on stderr: the envelope {"error":{"code","message"}}
// under --json, a line for people otherwise.
func (a *App) report(sys System, err error, jsonOut bool) {
	if jsonOut {
		envelope := map[string]map[string]string{"error": {"code": string(fail.CodeOf(err)), "message": err.Error()}}
		var data bytes.Buffer
		enc := json.NewEncoder(&data) // one line, < and > as is (as jq writes them)
		enc.SetEscapeHTML(false)
		if enc.Encode(envelope) == nil {
			_, _ = sys.Stderr.Write(data.Bytes())
			return
		}
	}
	w := ui.Writer(sys.Stderr, sys.Environ)
	fmt.Fprintf(w, "%s %s\n", ui.Frappe.Error.Render(a.Name+":"), err.Error())
}

func (a *App) version(sys System, rest []string) error {
	jsonOut := false
	for _, r := range rest {
		if r != "--json" || jsonOut {
			return fail.Usagef("--version takes no argument but --json: %s", r)
		}
		jsonOut = true
	}
	if jsonOut {
		return writeJSON(sys.Stdout, map[string]string{"version": a.Version})
	}
	_, err := fmt.Fprintf(sys.Stdout, "%s %s\n", a.Name, a.Version)
	return err
}

// commandIndex is what `relay --help --json` prints.
type commandIndex struct {
	Commands []indexEntry `json:"commands"`
}

type indexEntry struct {
	Command string `json:"command"`
	Summary string `json:"summary"`
}

// usage prints the list of commands: of every entity, of one, or of one
// group of verbs within it.
func (a *App) usage(sys System, jsonOut bool, only *Entity, group string) error {
	var cmds []*Command
	for _, c := range a.Commands() {
		if (only == nil || c.entity == only.Name) && (group == "" || strings.HasPrefix(c.Verb, group+" ")) {
			cmds = append(cmds, c)
		}
	}
	if jsonOut {
		index := commandIndex{Commands: []indexEntry{}}
		for _, c := range cmds {
			index.Commands = append(index.Commands, indexEntry{c.Name(), c.Summary})
		}
		return writeJSON(sys.Stdout, index)
	}
	t := ui.Frappe
	var b strings.Builder
	if only == nil {
		fmt.Fprintf(&b, "%s %s %s %s %s\n", t.Title.Render("Usage:"), t.Command.Render(a.Name),
			t.Arg.Render("<entity> <verb>"), t.Flag.Render("[options]"), t.Flag.Render("[--json[=<input>]]"))
	} else {
		name := strings.TrimSpace(a.Name + " " + only.Name + " " + group)
		fmt.Fprintf(&b, "%s %s %s %s\n\n%s\n", t.Title.Render("Usage:"), t.Command.Render(name),
			t.Arg.Render("<verb>"), t.Flag.Render("[options] [--json[=<input>]]"), only.Summary)
	}
	rows := make([][2]string, 0, len(cmds))
	for _, c := range cmds {
		rows = append(rows, [2]string{t.Command.Render(c.entity + " " + c.Verb), c.Summary})
	}
	if len(rows) > 0 {
		fmt.Fprintf(&b, "\n%s", table(rows))
	}
	fmt.Fprintf(&b, "\n%s\n", renderLines(t.Muted, a.Name+` <entity> <verb> --help: a command's help; with --json, its input spec.
`+a.Name+` --version: the version.
Exit codes: 0 success, 1 failure, 2 usage. With --json, an error is an object
{"error": {"code", "message"}} on stderr.`))
	_, err := io.WriteString(ui.Writer(sys.Stdout, sys.Environ), b.String())
	return err
}

func visibleWidth(s string) int { return lipgloss.Width(s) }
