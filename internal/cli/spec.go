// Package cli is relay's contract layer (docs/contract.md): a command is
// declared once, as a Spec and a run function taking a typed params struct;
// the layer derives from it the strict flag parser, the JSON input, the help
// and the input spec, the JSON output and the error envelope.
package cli

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
)

// Type is a param's type, as the input spec names it.
type Type string

// The param types. Each maps to a Go field type of the params struct.
const (
	String Type = "string"  // string
	Bool   Type = "boolean" // bool
	Int    Type = "integer" // int
	List   Type = "array"   // []string; comma-separated in the flag form
)

// DryRun is the param every state-changing command takes.
const DryRun = "dry_run"

// Param is one input of a command: the JSON key Name, the flag --name.
type Param struct {
	// Name is the JSON key, in snake_case; the flag writes _ as -
	// (dry_run, --dry-run). It matches the `json` tag of a field of the
	// params struct, whose Go type gives the param's Type.
	Name string
	// Help says what the param does, in one line or a few.
	Help string
	// Short is an optional one-letter alias of the flag: "R" for -R.
	Short string
	// Placeholder names the value in the help (<when>); Name by default.
	Placeholder string
	// Required makes the param mandatory, in both forms.
	Required bool
	// Default is the value when the param is absent, of the param's Go type.
	Default any
	// Enum lists the accepted values of a string or array param.
	Enum []string
	// Optional makes a string flag's value optional: --color alone means
	// Implicit, and a value can only be given with = (--color=never).
	Optional bool
	// Implicit is the value of an Optional flag given without a value.
	Implicit string

	typ   Type
	field int // index in the params struct; -1 for the layer's own params
}

// Type is the param's type, resolved from the params struct.
func (p *Param) Type() Type { return p.typ }

// Flag is the param's flag, --name with _ written as -.
func (p *Param) Flag() string { return "--" + strings.ReplaceAll(p.Name, "_", "-") }

func (p *Param) placeholder() string {
	if p.Placeholder != "" {
		return p.Placeholder
	}
	return strings.ReplaceAll(p.Name, "_", "-")
}

// Spec declares a command.
type Spec struct {
	// Verb is the command's name within its entity: relay <entity> <verb>.
	// Two kebab-case words group verbs under a noun of the entity:
	// "group hello" runs as relay fixture group hello.
	Verb string
	// Summary is one line, lowercase, for the command lists.
	Summary string
	// Help is the long help: what the command does, its side effects.
	Help string
	// Output describes the JSON output, as a shape: {"session": {"name", "path"}}.
	Output string
	// Params are the command's inputs, in the order the help lists them.
	Params []Param
	// Positional names the param the positional argument fills, if any; a
	// List param takes every positional argument.
	Positional string
	// Rest names the List param that takes the arguments after --.
	Rest string
	// StateChanging adds the dry_run param (--dry-run): the command then
	// validates and reports what it would do, without doing it.
	StateChanging bool
	// Interactive commands need a terminal: under --json they are refused
	// with tty_required, the message giving the command line to run.
	Interactive bool
	// DryRunJSON lets an interactive, state-changing command run its
	// --dry-run under --json: set it when the dry run asks nothing.
	DryRunJSON bool
	// Raw commands sit outside the JSON contract: their caller fixes the
	// interface (relay session run re-executes itself inside tmux with
	// claude's arguments). The params struct is Raw and receives the
	// arguments untouched; only a lone --help (with or without --json) is
	// read by the layer.
	Raw bool
}

// Raw is the params struct of a Raw command.
type Raw struct {
	Args []string
}

// RunFunc runs a command with its typed params. The result is printed on
// stdout (as JSON under --json), even when an error is returned too.
type RunFunc[P any] func(ctx context.Context, env *Env, p P) (any, error)

// Command is a declared command, ready to register in an Entity.
type Command struct {
	Spec
	entity   string
	app      string
	params   []Param // Spec.Params with their types, plus dry_run
	ptype    reflect.Type
	run      func(ctx context.Context, env *Env, p any) (any, error)
	problems []string
}

// New declares a command from its spec and its run function. Mistakes in the
// spec (a param without a field, a wrong default…) are reported by Problems,
// which the registry test checks for every command.
func New[P any](spec Spec, run RunFunc[P]) *Command {
	c := &Command{Spec: spec, ptype: reflect.TypeFor[P]()}
	c.run = func(ctx context.Context, env *Env, p any) (any, error) {
		v, ok := p.(P)
		if !ok {
			return nil, fmt.Errorf("internal: params of %s are %T, not %s", env.Command, p, c.ptype)
		}
		return run(ctx, env, v)
	}
	c.resolve()
	return c
}

// Entity groups the commands on one noun: relay <entity> <verb>.
type Entity struct {
	// Name is the noun, in kebab-case.
	Name string
	// Summary is one line, lowercase, for the entity list.
	Summary string
	// Commands are the entity's verbs, in the order the help lists them.
	Commands []*Command
}

// Name is the command's full name: relay <entity> <verb>.
func (c *Command) Name() string { return c.app + " " + c.entity + " " + c.Verb }

// Params are the command's inputs, the layer's own (dry_run) included.
func (c *Command) Params() []Param { return c.params }

func (c *Command) param(name string) *Param {
	for i := range c.params {
		if c.params[i].Name == name {
			return &c.params[i]
		}
	}
	return nil
}

func (c *Command) short(letter string) *Param {
	if letter == "" {
		return nil
	}
	for i := range c.params {
		if c.params[i].Short == letter {
			return &c.params[i]
		}
	}
	return nil
}

var (
	kebab  = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
	snake  = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	letter = regexp.MustCompile(`^[a-zA-Z]$`)
	verb   = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*( [a-z][a-z0-9]*(-[a-z0-9]+)*)?$`)
)

// resolve gives each param its type and field, then checks the spec.
func (c *Command) resolve() {
	bad := func(format string, args ...any) { c.problems = append(c.problems, fmt.Sprintf(format, args...)) }
	if !verb.MatchString(c.Verb) {
		bad("verb %q is not kebab-case, in one word or two", c.Verb)
	}
	if strings.TrimSpace(c.Summary) == "" {
		bad("no summary")
	}
	if strings.TrimSpace(c.Help) == "" {
		bad("no help")
	}
	if c.ptype.Kind() != reflect.Struct {
		bad("params type %s is not a struct", c.ptype)
		return
	}
	if c.Raw {
		c.resolveRaw(bad)
		return
	}
	if strings.TrimSpace(c.Output) == "" {
		bad("no output description")
	}
	fields := map[string]int{}
	for i := range c.ptype.NumField() {
		f := c.ptype.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if !f.IsExported() || name == "" || name == "-" {
			bad("field %s has no json tag", f.Name)
			continue
		}
		fields[name] = i
	}
	seen := map[string]bool{}
	shorts := map[string]bool{}
	for i := range c.Spec.Params {
		p := c.Spec.Params[i] // a copy, resolved into c.params
		switch {
		case !snake.MatchString(p.Name):
			bad("param %q is not snake_case", p.Name)
		case seen[p.Name]:
			bad("param %q declared twice", p.Name)
		case p.Name == DryRun || p.Name == "json" || p.Name == "help":
			bad("param %q is the layer's own", p.Name)
		}
		seen[p.Name] = true
		i, ok := fields[p.Name]
		if !ok {
			bad("param %q has no field with that json tag", p.Name)
			continue
		}
		delete(fields, p.Name)
		p.field = i
		p.typ = typeOf(c.ptype.Field(i).Type)
		if p.typ == "" {
			bad("param %q: field type %s is not string, bool, int or []string", p.Name, c.ptype.Field(i).Type)
			continue
		}
		c.checkParam(&p, bad)
		if p.Short != "" {
			if shorts[p.Short] {
				bad("short -%s declared twice", p.Short)
			}
			shorts[p.Short] = true
		}
		c.params = append(c.params, p)
	}
	for name := range fields {
		bad("field with json tag %q has no param", name)
	}
	if c.DryRunJSON && (!c.Interactive || !c.StateChanging) {
		bad("dry_run_json needs an interactive, state-changing command")
	}
	if c.StateChanging {
		c.params = append(c.params, Param{
			Name: DryRun, typ: Bool, field: -1,
			Help: "validates and reports what would be done, without doing it",
		})
	}
	if c.Positional != "" {
		if p := c.param(c.Positional); p == nil || p.field < 0 {
			bad("positional %q is not a param", c.Positional)
		} else if p.typ == Bool {
			bad("positional %q is a boolean", c.Positional)
		}
	}
	if c.Rest != "" {
		if p := c.param(c.Rest); p == nil || p.typ != List {
			bad("rest %q is not an array param", c.Rest)
		} else if c.Rest == c.Positional {
			bad("rest %q is also the positional", c.Rest)
		}
	}
}

func (c *Command) checkParam(p *Param, bad func(string, ...any)) {
	if strings.TrimSpace(p.Help) == "" {
		bad("param %q has no help", p.Name)
	}
	if p.Short != "" && (!letter.MatchString(p.Short) || p.Short == "h") {
		bad("param %q: short %q is not one letter other than h", p.Name, p.Short)
	}
	if p.Default != nil && reflect.TypeOf(p.Default) != c.ptype.Field(p.field).Type {
		bad("param %q: default %#v is not a %s", p.Name, p.Default, c.ptype.Field(p.field).Type)
	}
	if p.Required && p.Default != nil {
		bad("param %q is required and has a default", p.Name)
	}
	if len(p.Enum) > 0 {
		if p.typ != String && p.typ != List {
			bad("param %q: enum on a %s", p.Name, p.typ)
		}
		if d, ok := p.Default.(string); ok && !slices.Contains(p.Enum, d) {
			bad("param %q: default %q is not in its enum", p.Name, d)
		}
	}
	if p.Optional && p.typ != String {
		bad("param %q: only a string param can have an optional value", p.Name)
	}
	if !p.Optional && p.Implicit != "" {
		bad("param %q: implicit value without optional", p.Name)
	}
}

func (c *Command) resolveRaw(bad func(string, ...any)) {
	if c.ptype != reflect.TypeFor[Raw]() {
		bad("a raw command takes cli.Raw, not %s", c.ptype)
	}
	if len(c.Spec.Params) > 0 || c.Positional != "" || c.Rest != "" || c.StateChanging || c.Interactive {
		bad("a raw command declares no params, positional, rest, state_changing or interactive")
	}
}

func typeOf(t reflect.Type) Type {
	switch {
	case t.Kind() == reflect.String:
		return String
	case t.Kind() == reflect.Bool:
		return Bool
	case t.Kind() == reflect.Int:
		return Int
	case t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.String:
		return List
	default:
		return ""
	}
}

// Problems lists the mistakes in the entity's declaration and its commands'
// specs; empty when the entity is sound.
func (e *Entity) Problems() []string {
	var out []string
	if !kebab.MatchString(e.Name) {
		out = append(out, fmt.Sprintf("entity %q is not kebab-case", e.Name))
	}
	if strings.TrimSpace(e.Summary) == "" {
		out = append(out, fmt.Sprintf("entity %s has no summary", e.Name))
	}
	if len(e.Commands) == 0 {
		out = append(out, fmt.Sprintf("entity %s has no command", e.Name))
	}
	verbs := map[string]bool{}
	for _, c := range e.Commands {
		if group, _, ok := strings.Cut(c.Verb, " "); ok && e.command(group) != nil {
			out = append(out, fmt.Sprintf("%s %s: %s is both a verb and a group", e.Name, c.Verb, group))
		}
		if verbs[c.Verb] {
			out = append(out, fmt.Sprintf("%s %s declared twice", e.Name, c.Verb))
		}
		verbs[c.Verb] = true
		for _, p := range c.problems {
			out = append(out, fmt.Sprintf("%s %s: %s", e.Name, c.Verb, p))
		}
	}
	return out
}
