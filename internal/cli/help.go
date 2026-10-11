package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/FunDrivenDev/relay/internal/ui"
)

// InputSpec is what `<command> --help --json` prints: each JSON key and its
// type, and how the flag form maps onto them.
type InputSpec struct {
	Command       string            `json:"command"`
	Summary       string            `json:"summary"`
	Input         ordered           `json:"input"`
	Positional    string            `json:"positional,omitempty"`
	Rest          string            `json:"rest,omitempty"`
	Short         map[string]string `json:"short,omitempty"`
	Required      []string          `json:"required,omitempty"`
	Defaults      ordered           `json:"defaults,omitempty"`
	Enum          ordered           `json:"enum,omitempty"`
	Implicit      ordered           `json:"implicit,omitempty"`
	Help          ordered           `json:"help,omitempty"`
	Output        string            `json:"output,omitempty"`
	StateChanging bool              `json:"state_changing"`
	Interactive   bool              `json:"interactive"`
	DryRunJSON    bool              `json:"dry_run_json,omitempty"`
	Raw           bool              `json:"raw,omitempty"`
}

// ordered is a JSON object that keeps its keys in declaration order.
type ordered []field

type field struct {
	key   string
	value any
}

func (o ordered) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, f := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		k, err := json.Marshal(f.key)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(f.value)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// InputSpec returns the command's input spec.
func (c *Command) InputSpec() InputSpec {
	s := InputSpec{
		Command: c.Name(), Summary: c.Summary, Output: c.Output,
		Input: ordered{}, Positional: c.Positional, Rest: c.Rest,
		StateChanging: c.StateChanging, Interactive: c.Interactive, DryRunJSON: c.DryRunJSON, Raw: c.Raw,
	}
	for i := range c.params {
		p := &c.params[i]
		s.Input = append(s.Input, field{p.Name, p.typ})
		if p.Short != "" {
			if s.Short == nil {
				s.Short = map[string]string{}
			}
			s.Short[p.Short] = p.Name
		}
		if p.Required {
			s.Required = append(s.Required, p.Name)
		}
		if p.Default != nil {
			s.Defaults = append(s.Defaults, field{p.Name, p.Default})
		}
		if len(p.Enum) > 0 {
			s.Enum = append(s.Enum, field{p.Name, p.Enum})
		}
		if p.Optional {
			s.Implicit = append(s.Implicit, field{p.Name, p.Implicit})
		}
		s.Help = append(s.Help, field{p.Name, p.Help})
	}
	return s
}

// Help renders the command's human help.
func (c *Command) help(t ui.Theme) string {
	var b strings.Builder
	name := t.Command.Render(c.Name())
	if c.Raw {
		fmt.Fprintf(&b, "%s %s %s\n\n%s\n", t.Title.Render("Usage:"), name, t.Arg.Render("<arguments…>"), strings.TrimRight(c.Help, "\n"))
		fmt.Fprintf(&b, "\n%s\n", t.Muted.Render("Outside the JSON contract: the arguments go to the command untouched."))
		return b.String()
	}
	fmt.Fprintf(&b, "%s %s%s\n", t.Title.Render("Usage:"), name, c.synopsis(t))
	fmt.Fprintf(&b, "       %s %s\n", name, t.Flag.Render("--json="+c.jsonExample()))
	fmt.Fprintf(&b, "\n%s\n", strings.TrimRight(c.Help, "\n"))
	if c.Interactive {
		note := "Interactive: refused under --json (code tty_required), the message giving the command to run."
		if c.DryRunJSON {
			note = "Interactive: refused under --json (code tty_required), the message giving the command to run;\n--dry-run asks nothing and runs under --json."
		}
		fmt.Fprintf(&b, "\n%s\n", renderLines(t.Warn, note))
	}

	rows := [][2]string{}
	for i := range c.params {
		p := &c.params[i]
		rows = append(rows, [2]string{c.optionLabel(t, p), c.optionHelp(t, p)})
	}
	output := "JSON output"
	if c.Output != "" {
		output += ": " + c.Output
	}
	rows = append(rows,
		[2]string{t.Flag.Render("--json"), output + "\n--json=<object>, or --json=- (stdin): the inputs, keys = option names in snake_case"},
		[2]string{t.Flag.Render("-h") + ", " + t.Flag.Render("--help"), "this help; with --json, the input spec"},
	)
	fmt.Fprintf(&b, "\n%s\n%s", t.Title.Render("Options:"), table(rows))
	fmt.Fprintf(&b, "\n%s\n", renderLines(t.Muted, c.notes()))
	return b.String()
}

func (c *Command) notes() string {
	notes := []string{"A value option takes the next argument, unless it starts with -: then write --option=<value>."}
	for i := range c.params {
		if p := &c.params[i]; p.typ == List && p.Name != c.Rest && p.Name != c.Positional {
			notes = append(notes, "A list option takes comma-separated values: --option=a,b.")
			break
		}
	}
	return strings.Join(notes, "\n")
}

func (c *Command) synopsis(t ui.Theme) string {
	var b strings.Builder
	for i := range c.params {
		p := &c.params[i]
		if p.Name == c.Positional || p.Name == c.Rest {
			continue
		}
		item := t.Flag.Render(p.Flag())
		switch {
		case p.typ == Bool:
		case p.Optional:
			item += "[=" + t.Arg.Render("<"+p.placeholder()+">") + "]"
		default:
			item += " " + t.Arg.Render("<"+p.placeholder()+">")
		}
		if !p.Required {
			item = "[" + item + "]"
		}
		b.WriteString(" " + item)
	}
	if p := c.param(c.Positional); p != nil {
		item := t.Arg.Render("<" + p.placeholder() + ">")
		if p.typ == List {
			item += "…"
		}
		if !p.Required {
			item = "[" + item + "]"
		}
		b.WriteString(" " + item)
	}
	if p := c.param(c.Rest); p != nil {
		b.WriteString(" [" + t.Flag.Render("--") + " " + t.Arg.Render("<"+p.placeholder()+">") + "…]")
	}
	return b.String()
}

func (c *Command) optionLabel(t ui.Theme, p *Param) string {
	placeholder := t.Arg.Render("<" + p.placeholder() + ">")
	switch p.Name {
	case c.Rest:
		return t.Flag.Render("--") + " " + placeholder + "…"
	case c.Positional:
		return placeholder + ", " + t.Flag.Render(p.Flag())
	}
	label := t.Flag.Render(p.Flag())
	if p.Short != "" {
		label = t.Flag.Render("-"+p.Short) + ", " + label
	}
	switch {
	case p.typ == Bool:
	case p.Optional:
		label += "[=" + placeholder + "]"
	default:
		label += " " + placeholder
	}
	return label
}

func (c *Command) optionHelp(t ui.Theme, p *Param) string {
	var extra []string
	if p.Required {
		extra = append(extra, "required")
	}
	if len(p.Enum) > 0 {
		extra = append(extra, "one of: "+strings.Join(p.Enum, ", "))
	}
	if p.Default != nil {
		extra = append(extra, "default: "+fmt.Sprint(formatDefault(p.Default)))
	}
	if p.Optional {
		extra = append(extra, "alone: "+p.Implicit)
	}
	help := strings.TrimRight(p.Help, "\n")
	if len(extra) > 0 {
		help += " " + t.Muted.Render("("+strings.Join(extra, "; ")+")")
	}
	return help
}

func formatDefault(v any) any {
	if l, ok := v.([]string); ok {
		return strings.Join(l, ",")
	}
	return v
}

// jsonExample is the --json=<object> line of the usage: the required keys,
// or the first one, with a placeholder value.
func (c *Command) jsonExample() string {
	var keys []string
	more := false
	anyRequired := slices.ContainsFunc(c.params, func(q Param) bool { return q.Required })
	for i := range c.params {
		p := &c.params[i]
		if p.Required || (len(keys) == 0 && !anyRequired) {
			keys = append(keys, fmt.Sprintf("%q: %s", p.Name, placeholderValue(p.typ)))
		} else {
			more = true
		}
	}
	if more {
		keys = append(keys, "…")
	}
	return "'{" + strings.Join(keys, ", ") + "}'"
}

func placeholderValue(t Type) string {
	switch t {
	case Bool:
		return "true"
	case Int:
		return "0"
	case List:
		return `["…"]`
	default:
		return `"…"`
	}
}

// renderLines styles each line of s on its own: lipgloss pads a multi-line
// block to its widest line.
func renderLines(style lipgloss.Style, s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = style.Render(l)
	}
	return strings.Join(lines, "\n")
}

// table aligns rows of two columns; a second column on several lines keeps
// its indent.
func table(rows [][2]string) string {
	width := 0
	for _, r := range rows {
		width = max(width, visibleWidth(r[0]))
	}
	var b strings.Builder
	for _, r := range rows {
		lines := strings.Split(r[1], "\n")
		pad := strings.Repeat(" ", width-visibleWidth(r[0])+3)
		fmt.Fprintf(&b, "  %s%s%s\n", r[0], pad, lines[0])
		for _, l := range lines[1:] {
			fmt.Fprintf(&b, "  %s%s\n", strings.Repeat(" ", width+3), l)
		}
	}
	return b.String()
}
