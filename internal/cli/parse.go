package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/FunDrivenDev/relay/internal/fail"
)

// Values are a command's inputs once read, whichever the form: key → string,
// bool, int or []string. Both the flags and --json=<object> build them, then
// the same code checks and binds them.
type Values map[string]any

// invocation is a parsed command line.
type invocation struct {
	values   Values
	json     bool // --json: JSON output (and errors)
	help     bool // --help or -h
	fromJSON bool // the inputs came from --json=<object|->
}

// wantsJSON reports whether --json (in any form) is among args, before any --:
// errors are then JSON too, even one raised before the parser reaches it.
func wantsJSON(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == "--json" || strings.HasPrefix(a, "--json=") {
			return true
		}
	}
	return false
}

func wantsHelp(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == "--help" || a == "-h" {
			return true
		}
	}
	return false
}

// parse reads a command's arguments (after relay <entity> <verb>).
func (c *Command) parse(args []string, stdin io.Reader) (invocation, error) {
	inv := invocation{json: wantsJSON(args), values: Values{}}
	if c.Raw {
		return c.parseRaw(args)
	}
	if wantsHelp(args) {
		inv.help = true
		return inv, nil
	}
	var payload *string
	sawJSON, flags := false, false
	// A list positional, given as arguments and as its option: one or the
	// other (a single positional is refused as a repeated one).
	byArg, byFlag := false, false
	listPositional := c.Positional != "" && c.param(c.Positional).typ == List
	both := func() error {
		p := c.param(c.Positional)
		return fail.Usagef("<%s> and %s: one or the other", p.placeholder(), p.Flag())
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			if c.Rest == "" {
				return inv, fail.Usagef("no arguments are accepted after --")
			}
			inv.values[c.Rest] = slices.Clone(args[i+1:])
			flags = true
			i = len(args)
		case a == "--json" || strings.HasPrefix(a, "--json="):
			if sawJSON {
				return inv, fail.Usagef("repeated option: --json")
			}
			sawJSON = true
			if v, ok := strings.CutPrefix(a, "--json="); ok {
				payload = &v
			}
		case strings.HasPrefix(a, "--"):
			name, value, hasValue := strings.Cut(a[2:], "=")
			p := c.param(strings.ReplaceAll(name, "-", "_"))
			if strings.Contains(name, "_") || p == nil || p.Name == c.Rest {
				return inv, fail.Usagef("unknown option: --%s", name)
			}
			next, consumed, err := c.flagValue(p, "--"+name, value, hasValue, args[i+1:])
			if err != nil {
				return inv, err
			}
			i += consumed
			if listPositional && p.Name == c.Positional {
				if byArg {
					return inv, both()
				}
				byFlag = true
			}
			if err := c.setFlag(inv.values, p, "--"+name, next); err != nil {
				return inv, err
			}
			flags = true
		case strings.HasPrefix(a, "-"):
			p := c.short(strings.TrimPrefix(a, "-"))
			if p == nil {
				return inv, fail.Usagef("unknown option: %s", a)
			}
			next, consumed, err := c.flagValue(p, a, "", false, args[i+1:])
			if err != nil {
				return inv, err
			}
			i += consumed
			if listPositional && p.Name == c.Positional {
				if byArg {
					return inv, both()
				}
				byFlag = true
			}
			if err := c.setFlag(inv.values, p, a, next); err != nil {
				return inv, err
			}
			flags = true
		default:
			if listPositional && byFlag {
				return inv, both()
			}
			byArg = true
			if err := c.setPositional(inv.values, a); err != nil {
				return inv, err
			}
			flags = true
		}
	}
	if payload != nil {
		if flags {
			return inv, fail.Usagef("--json=<input> carries all the inputs: no other option or argument allowed")
		}
		values, err := c.decodeJSON(*payload, stdin)
		if err != nil {
			return inv, err
		}
		inv.values, inv.fromJSON = values, true
	}
	return inv, c.complete(inv.values, inv.fromJSON)
}

func (c *Command) parseRaw(args []string) (invocation, error) {
	// Help is a lone --help (or -h), with or without --json.
	help, jsonOut, other := 0, 0, 0
	for _, a := range args {
		switch a {
		case "--help", "-h":
			help++
		case "--json":
			jsonOut++
		default:
			other++
		}
	}
	if help == 1 && jsonOut <= 1 && other == 0 {
		return invocation{help: true, json: jsonOut == 1}, nil
	}
	return invocation{values: Values{"args": slices.Clone(args)}}, nil
}

// flagValue reads the value of flag p from its =value or from the next
// argument, and reports how many arguments it consumed.
func (c *Command) flagValue(p *Param, flag, value string, hasValue bool, rest []string) (string, int, error) {
	switch {
	case p.typ == Bool:
		if hasValue {
			return "", 0, fail.Usagef("%s takes no value", flag)
		}
		return "", 0, nil
	case hasValue:
		return value, 0, nil
	case p.Optional:
		return p.Implicit, 0, nil
	case len(rest) > 0 && !strings.HasPrefix(rest[0], "-"):
		return rest[0], 1, nil
	default:
		return "", 0, fail.Usagef("%s expects a value (write %s=<value> if it starts with -)", flag, p.Flag())
	}
}

func (c *Command) setFlag(values Values, p *Param, flag, raw string) error {
	if _, ok := values[p.Name]; ok {
		return fail.Usagef("repeated option: %s", flag)
	}
	switch p.typ {
	case Bool:
		values[p.Name] = true
	case Int:
		n, err := strconv.Atoi(raw)
		if err != nil {
			return fail.Usagef("%s expects an integer, not %q", flag, raw)
		}
		values[p.Name] = n
	case List:
		values[p.Name] = splitList(raw)
	default:
		values[p.Name] = raw
	}
	return nil
}

func splitList(raw string) []string {
	if raw == "" {
		return []string{}
	}
	return strings.Split(raw, ",")
}

func (c *Command) setPositional(values Values, a string) error {
	if c.Positional == "" {
		return fail.Usagef("unexpected argument: %s", a)
	}
	p := c.param(c.Positional)
	current, ok := values[p.Name]
	switch {
	case p.typ == List && ok:
		values[p.Name] = append(current.([]string), a) //nolint:forcetypeassert // a List param always holds []string
	case p.typ == List:
		values[p.Name] = []string{a}
	case ok:
		return fail.Usagef("only one positional argument (<%s>); extra: %s", p.placeholder(), a)
	case p.typ == Int:
		n, err := strconv.Atoi(a)
		if err != nil {
			return fail.Usagef("<%s> expects an integer, not %q", p.placeholder(), a)
		}
		values[p.Name] = n
	default:
		values[p.Name] = a
	}
	return nil
}

// decodeJSON reads --json=<object> (--json=- reads it from stdin) into values,
// checking each key and its type against the spec.
func (c *Command) decodeJSON(payload string, stdin io.Reader) (Values, error) {
	if payload == "-" {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fail.Wrap(fail.Failure, err, "reading the JSON input from stdin: %v", err)
		}
		payload = string(data)
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(payload)))
	dec.UseNumber()
	var object map[string]any
	if err := dec.Decode(&object); err != nil || object == nil {
		return nil, fail.Usagef("--json expects a JSON object")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fail.Usagef("--json expects a single JSON object")
	}
	keys := make([]string, 0, len(object))
	for k := range object {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	values := Values{}
	for _, k := range keys {
		p := c.param(k)
		if p == nil {
			return nil, fail.Usagef("unknown key: %s", k)
		}
		v, ok := fromJSON(p.typ, object[k])
		if !ok {
			return nil, fail.Usagef("%s: expected %s", k, describe(p.typ))
		}
		values[k] = v
	}
	return values, nil
}

func describe(t Type) string {
	switch t {
	case List:
		return "an array of strings"
	case Int:
		return "an integer"
	default:
		return "a " + string(t)
	}
}

func fromJSON(t Type, v any) (any, bool) {
	switch t {
	case String:
		s, ok := v.(string)
		return s, ok
	case Bool:
		b, ok := v.(bool)
		return b, ok
	case Int:
		n, ok := v.(json.Number)
		if !ok {
			return nil, false
		}
		i, err := strconv.Atoi(n.String())
		return i, err == nil
	case List:
		items, ok := v.([]any)
		if !ok {
			return nil, false
		}
		out := make([]string, 0, len(items))
		for _, item := range items {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	}
	return nil, false
}

// complete checks the required params and the enums, and fills the defaults:
// the last step both forms share before binding.
func (c *Command) complete(values Values, fromJSON bool) error {
	for i := range c.params {
		p := &c.params[i]
		v, ok := values[p.Name]
		switch {
		case !ok && p.Required:
			return fail.Usagef("missing %s", c.inputName(p, fromJSON))
		case !ok && p.Default != nil:
			values[p.Name] = p.Default
		case ok && len(p.Enum) > 0:
			if err := checkEnum(p, v, c.inputName(p, fromJSON)); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkEnum(p *Param, v any, name string) error {
	var given []string
	switch v := v.(type) {
	case string:
		given = []string{v}
	case []string:
		given = v
	}
	for _, g := range given {
		if !slices.Contains(p.Enum, g) {
			return fail.Usagef("%s: %q is not one of %s", name, g, strings.Join(p.Enum, ", "))
		}
	}
	return nil
}

// inputName names a param as the user wrote it: a key, a flag or <positional>.
func (c *Command) inputName(p *Param, fromJSON bool) string {
	switch {
	case fromJSON:
		return "key " + p.Name
	case p.Name == c.Positional:
		return "<" + p.placeholder() + ">"
	default:
		return p.Flag()
	}
}

// bind builds the params struct from complete values.
func (c *Command) bind(values Values) any {
	if c.Raw {
		args, _ := values["args"].([]string)
		return Raw{Args: args}
	}
	ptr := reflect.New(c.ptype)
	for i := range c.params {
		p := &c.params[i]
		v, ok := values[p.Name]
		if !ok || p.field < 0 {
			continue
		}
		field := ptr.Elem().Field(p.field)
		field.Set(reflect.ValueOf(v).Convert(field.Type()))
	}
	return ptr.Elem().Interface()
}

// dryRun reports whether the values ask for a dry run.
func (v Values) dryRun() bool {
	b, _ := v[DryRun].(bool)
	return b
}

// String renders values for messages and tests.
func (v Values) String() string {
	data, err := json.Marshal(map[string]any(v))
	if err != nil {
		return fmt.Sprint(map[string]any(v))
	}
	return string(data)
}
