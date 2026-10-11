// Package ui styles what relay prints for people: Catppuccin Frappé through
// lipgloss, downsampled to what the terminal supports and stripped when the
// output is not a terminal or NO_COLOR is set.
package ui

import (
	"io"
	"os"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"golang.org/x/term"
)

// Catppuccin Frappé, the colours relay uses.
var (
	Text     = lipgloss.Color("#c6d0f5")
	Subtext0 = lipgloss.Color("#a5adce")
	Overlay1 = lipgloss.Color("#838ba7")
	Mauve    = lipgloss.Color("#ca9ee6")
	Blue     = lipgloss.Color("#8caaee")
	Green    = lipgloss.Color("#a6d189")
	Yellow   = lipgloss.Color("#e5c890")
	Peach    = lipgloss.Color("#ef9f76")
	Red      = lipgloss.Color("#e78284")
)

// Theme holds the styles of relay's human output.
type Theme struct {
	Title   lipgloss.Style // a section heading ("Usage:")
	Command lipgloss.Style // a command or an example to type
	Flag    lipgloss.Style // an option name
	Arg     lipgloss.Style // a placeholder such as <name>
	Muted   lipgloss.Style // secondary text: defaults, notes
	OK      lipgloss.Style
	Warn    lipgloss.Style
	Error   lipgloss.Style
	// Rich is set when the output goes to a terminal that takes colours:
	// a result may then draw a table or a box rather than plain lines.
	Rich bool
}

// Frappe is the relay theme.
var Frappe = Theme{
	Title:   lipgloss.NewStyle().Foreground(Mauve).Bold(true),
	Command: lipgloss.NewStyle().Foreground(Blue),
	Flag:    lipgloss.NewStyle().Foreground(Green),
	Arg:     lipgloss.NewStyle().Foreground(Peach),
	Muted:   lipgloss.NewStyle().Foreground(Overlay1),
	OK:      lipgloss.NewStyle().Foreground(Green),
	Warn:    lipgloss.NewStyle().Foreground(Yellow),
	Error:   lipgloss.NewStyle().Foreground(Red).Bold(true),
}

// Writer wraps w so that the styles written to it fit the terminal behind w:
// plain text when w is not a terminal, or when NO_COLOR is set in environ.
func Writer(w io.Writer, environ []string) io.Writer {
	return colorprofile.NewWriter(w, environ)
}

// IsRich reports whether w is a terminal that takes styles: not when NO_COLOR
// is set (not empty) or TERM is dumb in environ.
func IsRich(w io.Writer, environ []string) bool {
	f, ok := w.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return false
	}
	return !slices.ContainsFunc(environ, func(kv string) bool {
		k, v, _ := strings.Cut(kv, "=")
		return (k == "NO_COLOR" && v != "") || (k == "TERM" && v == "dumb")
	})
}

// For is the theme for output written to w: Frappe, Rich when w takes it.
func For(w io.Writer, environ []string) Theme {
	t := Frappe
	t.Rich = IsRich(w, environ)
	return t
}
