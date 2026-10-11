package session

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"

	"github.com/FunDrivenDev/relay/internal/cli"
	"github.com/FunDrivenDev/relay/internal/run"
	"github.com/FunDrivenDev/relay/internal/ui"
)

type listParams struct{}

type sessionRow struct {
	Name     string `json:"name"`
	Windows  int    `json:"windows"`
	Attached bool   `json:"attached"`
	Path     string `json:"path"`
}

type listResult struct {
	Sessions []sessionRow `json:"sessions"`
}

// Human draws a table on a colour terminal, one plain line per session
// otherwise (a pipe, NO_COLOR).
func (r listResult) Human(t ui.Theme) string {
	if t.Rich && len(r.Sessions) > 0 {
		return r.table(t)
	}
	var b strings.Builder
	for _, s := range r.Sessions {
		attached := ""
		if s.Attached {
			attached = "  (attached)"
		}
		fmt.Fprintf(&b, "%s  %d window(s)%s  %s\n", s.Name, s.Windows, attached, s.Path)
	}
	return b.String()
}

// table renders the sessions as a lipgloss table in Catppuccin Frappé.
func (r listResult) table(t ui.Theme) string {
	header := t.Title.Padding(0, 1)
	cell := lipgloss.NewStyle().Padding(0, 1).Foreground(ui.Text)
	tbl := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(ui.Overlay1)).
		Headers("SESSION", "WINDOWS", "ATTACHED", "PATH").
		StyleFunc(func(row, col int) lipgloss.Style {
			switch {
			case row == table.HeaderRow:
				return header
			case col == 0:
				return cell.Foreground(ui.Blue)
			case col == 2 && r.Sessions[row].Attached:
				return cell.Foreground(ui.Green)
			case col == 3:
				return cell.Foreground(ui.Subtext0)
			default:
				return cell
			}
		})
	for _, s := range r.Sessions {
		attached := "no"
		if s.Attached {
			attached = "yes"
		}
		tbl.Row(s.Name, strconv.Itoa(s.Windows), attached, s.Path)
	}
	return tbl.String() + "\n"
}

var list = cli.New(cli.Spec{
	Verb:    "list",
	Summary: "lists the sessions",
	Help: `Lists the tmux sessions, to attach to by name: one line each, with its
windows, whether a client is attached, and the current path of its active
pane. No tmux server running is no session.`,
	Output: `{"sessions": [{"name", "windows", "attached", "path"}]}`,
}, func(ctx context.Context, env *cli.Env, _ listParams) (any, error) {
	res, err := run.Tool(ctx, "tmux", "list-sessions", "-F", listFormat)
	if err != nil {
		res.Stdout = ""
	}
	out := listResult{Sessions: parseSessions(res.Stdout)}
	if len(out.Sessions) == 0 {
		env.Say("%s", env.Theme.Muted.Render("no sessions"))
	}
	return out, nil
})

// listFormat separates the fields with ":", which tmux keeps out of session
// names, and ends on the path, which may hold anything: tmux prints a tab (any
// control character) in -F output as "_".
const listFormat = "#{session_name}:#{session_windows}:#{session_attached}:#{pane_current_path}"

// parseSessions reads tmux list-sessions rows (listFormat): name, windows,
// attached clients, path.
func parseSessions(rows string) []sessionRow {
	out := []sessionRow{}
	for line := range strings.SplitSeq(rows, "\n") {
		if line == "" {
			continue
		}
		f := strings.SplitN(line, ":", 4)
		for len(f) < 4 {
			f = append(f, "")
		}
		windows, _ := strconv.Atoi(f[1])
		clients, _ := strconv.Atoi(f[2])
		out = append(out, sessionRow{Name: f[0], Windows: windows, Attached: clients > 0, Path: f[3]})
	}
	return out
}
