package session

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/FunDrivenDev/relay/internal/cli"
	"github.com/FunDrivenDev/relay/internal/fail"
	"github.com/FunDrivenDev/relay/internal/ui"
)

func TestParseRelay(t *testing.T) {
	t.Parallel()
	o, err := parseRelay([]string{"--model", "opus", "--afk", "--not-a-flag", "--yolo", "x", "--until", "8"})
	if err != nil {
		t.Fatal(err)
	}
	if o.goal != "--not-a-flag" || o.until != "8" || !o.yolo || !slices.Equal(o.args, []string{"--model", "opus", "x"}) {
		t.Errorf("got %+v", o)
	}
	if got := o.afk(); !slices.Equal(got, []string{"--afk", "--not-a-flag", "--until", "8", "--yolo"}) {
		t.Errorf("afk() = %q", got)
	}
	for _, argv := range [][]string{{"--until", "8"}, {"--yolo"}, {"--afk"}, {"--afk", ""}, {"--afk", "g", "--until"}} {
		if _, err := parseRelay(argv); fail.CodeOf(err) != fail.Usage {
			t.Errorf("%q: got %v, want a usage error", argv, err)
		}
	}
}

func TestAfkSessionName(t *testing.T) {
	t.Parallel()
	taken := map[string]bool{"afk-my-proj-x": true, "afk-my-proj-x-2": true}
	if got := afkSessionName("/a/my.proj:x", func(n string) bool { return taken[n] }); got != "afk-my-proj-x-3" {
		t.Errorf("got %s", got)
	}
	if got := afkSessionName("/a/b", func(string) bool { return false }); got != "afk-b" {
		t.Errorf("got %s", got)
	}
}

func TestSplitSettings(t *testing.T) {
	t.Parallel()
	user, rest, err := splitSettings([]string{"--settings", "a.json", "-p", "--settings={}", "x"})
	if err != nil || user != "{}" || !slices.Equal(rest, []string{"-p", "x"}) {
		t.Errorf("got %q %q %v", user, rest, err)
	}
	if _, _, err := splitSettings([]string{"--settings"}); fail.CodeOf(err) != fail.Usage {
		t.Errorf("got %v", err)
	}
}

func TestCreateRelayArgsAndWhere(t *testing.T) {
	t.Parallel()
	p := createParams{Prompt: "goal", Afk: true, Until: "8", Yolo: true, Args: []string{"--model", "opus"}}
	if got := createRelayArgs(p); !slices.Equal(got, []string{"--afk", "goal", "--until", "8", "--yolo", "--model", "opus"}) {
		t.Errorf("got %q", got)
	}
	p = createParams{Prompt: "hi", Args: []string{"-c"}}
	if got := createRelayArgs(p); !slices.Equal(got, []string{"-c", "hi"}) {
		t.Errorf("got %q", got)
	}
	cases := []struct {
		p      createParams
		inTmux bool
		want   string
	}{
		{createParams{Name: "n", Detach: true}, true, whereDetached},
		{createParams{Name: "n"}, true, whereCurrent},
		{createParams{Name: "n"}, false, whereAttached},
		{createParams{}, true, wherePane},
		{createParams{}, false, whereTab},
	}
	for _, c := range cases {
		if got := whereOf(c.p, c.inTmux); got != c.want {
			t.Errorf("%+v in tmux %v: got %s, want %s", c.p, c.inTmux, got, c.want)
		}
	}
}

func TestParseSessions(t *testing.T) {
	t.Parallel()
	got := parseSessions("a\t1\t0\t/x\nb\t2\t1\t/y z\n\n")
	want := []sessionRow{{"a", 1, false, "/x"}, {"b", 2, true, "/y z"}}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v", got)
	}
	if got := parseSessions(""); got == nil || len(got) != 0 {
		t.Errorf("got %#v, want an empty list", got)
	}
}

func TestHandoverPy(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	env := &cli.Env{Environ: []string{"CLAUDE_CONFIG_DIR=" + dir}}
	if _, err := handoverPy(env); fail.CodeOf(err) != fail.RelayMissing {
		t.Fatalf("no installed_plugins.json: got %v", err)
	}
	plugins := filepath.Join(dir, "plugins")
	root := filepath.Join(dir, "cache", "handover", "1.2.3")
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(plugins, 0o755); err != nil {
		t.Fatal(err)
	}
	installed := `{"plugins": {"handover@fundrivendev": [{"installPath": "` + root + `"}]}}`
	if err := os.WriteFile(filepath.Join(plugins, "installed_plugins.json"), []byte(installed), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := handoverPy(env); fail.CodeOf(err) != fail.RelayMissing {
		t.Fatalf("no handover.py: got %v", err)
	}
	py := filepath.Join(root, "scripts", "handover.py")
	if err := os.WriteFile(py, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := handoverPy(env); err != nil || got != py {
		t.Errorf("got %s, %v", got, err)
	}
	home := &cli.Env{Environ: []string{"HOME=" + filepath.Join(dir, "nohome")}}
	if _, err := handoverPy(home); fail.CodeOf(err) != fail.RelayMissing {
		t.Errorf("HOME without .claude: got %v", err)
	}
}

func TestListHuman(t *testing.T) {
	t.Parallel()
	r := listResult{Sessions: []sessionRow{{"s1", 1, false, "/x"}, {"afk", 2, true, "/y"}}}
	plain := r.Human(ui.Frappe)
	if plain != "s1  1 window(s)  /x\nafk  2 window(s)  (attached)  /y\n" {
		t.Errorf("plain rows: %q", plain)
	}
	rich := ui.Frappe
	rich.Rich = true
	got := ansi.Strip(r.Human(rich))
	for _, want := range []string{"╭", "SESSION", "WINDOWS", "ATTACHED", "PATH", "│ s1 ", "│ afk ", "│ yes "} {
		if !strings.Contains(got, want) {
			t.Errorf("table lacks %q:\n%s", want, got)
		}
	}
	if (listResult{Sessions: []sessionRow{}}).Human(rich) != "" {
		t.Error("no sessions: no table")
	}
}
