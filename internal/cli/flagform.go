package cli

import (
	"reflect"
	"strconv"
	"strings"
)

// FlagForm writes values back as a command line, the flag form of the same
// input: the command a tty_required message tells to run in a terminal.
// Defaults are left out.
func (c *Command) FlagForm(values Values) string {
	parts := []string{c.Name()}
	var positional, rest []string
	for i := range c.params {
		p := &c.params[i]
		v, ok := values[p.Name]
		if !ok || (p.Default != nil && reflect.DeepEqual(v, p.Default)) {
			continue
		}
		switch {
		case p.Name == c.Rest:
			rest = append(rest, v.([]string)...) //nolint:forcetypeassert // a List param always holds []string
		case p.Name == c.Positional:
			positional = append(positional, textValues(v)...)
		case p.typ == Bool:
			if b, _ := v.(bool); b {
				parts = append(parts, p.Flag())
			}
		default:
			value := strings.Join(textValues(v), ",")
			if p.Optional || value == "" || strings.HasPrefix(value, "-") {
				parts = append(parts, quote(p.Flag()+"="+value))
			} else {
				parts = append(parts, p.Flag(), quote(value))
			}
		}
	}
	for _, v := range positional {
		if strings.HasPrefix(v, "-") {
			// A positional argument never starts with -: give it as its flag.
			parts = append(parts, quote(c.param(c.Positional).Flag()+"="+v))
			continue
		}
		parts = append(parts, quote(v))
	}
	if len(rest) > 0 {
		parts = append(parts, "--")
		for _, v := range rest {
			parts = append(parts, quote(v))
		}
	}
	return strings.Join(parts, " ")
}

func textValues(v any) []string {
	switch v := v.(type) {
	case []string:
		return v
	case int:
		return []string{strconv.Itoa(v)}
	case string:
		return []string{v}
	}
	return nil
}

// quote quotes s for a POSIX shell when it needs it.
func quote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && !strings.ContainsRune("-_./=:,@%+", r)
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
