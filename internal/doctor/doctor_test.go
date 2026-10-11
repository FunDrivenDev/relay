package doctor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/FunDrivenDev/relay/internal/cli"
)

func TestParseVersion(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   string
		want version
		ok   bool
	}{
		{"tmux 3.4", version{3, 4}, true},
		{"tmux 3.2a", version{3, 2}, true},
		{"tmux next-3.6", version{3, 6}, true},
		{"Python 3.12.3", version{3, 12}, true},
		{"tmux master", version{}, false},
	} {
		got, ok := parseVersion(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("parseVersion(%q) = %v, %v; want %v, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
	if !(version{3, 0}).less(tmuxFloor) || (version{3, 2}).less(tmuxFloor) || (version{10, 0}).less(pythonFloor) {
		t.Error("less disagrees with the floors")
	}
}

func TestToolVersion(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"tmux 3.2a":                    "3.2a",
		"tmux next-3.6":                "3.6",
		"2.1.296 (Claude Code)":        "2.1.296",
		"git version 2.43.0\nmore 1.0": "2.43.0",
		"ps from procps-ng 4.0.4":      "4.0.4",
		"notify-send":                  "",
	} {
		if got := toolVersion(in); got != want {
			t.Errorf("toolVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOSRelease(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "os-release")
	env := &cli.Env{Environ: []string{"RELAY_OS_RELEASE=" + path}}
	write := func(s string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("# comment\nNAME=\"Ubuntu\"\nVERSION='24.04 LTS'\nPRETTY_NAME=\"Ubuntu 24.04 LTS\"\n")
	if got := osRelease(env); got != "Ubuntu 24.04 LTS" {
		t.Errorf("PRETTY_NAME: got %q", got)
	}
	write("NAME=Debian\nVERSION=\"12 (bookworm)\"\n")
	if got := osRelease(env); got != "Debian 12 (bookworm)" {
		t.Errorf("NAME VERSION: got %q", got)
	}
	env.Environ = []string{"RELAY_OS_RELEASE=" + filepath.Join(dir, "none")}
	if got := osRelease(env); got != "" {
		t.Errorf("missing file: got %q", got)
	}
}

func TestPlan(t *testing.T) {
	env := &cli.Env{Environ: []string{"RELAY_EUID=0", "HOME=" + t.TempDir(), "PATH="}}
	t.Setenv("PATH", "")
	checks := []Check{
		{Name: "tmux", Required: true, step: stepApt, pkg: "tmux", Fix: fixWith("x")},
		{Name: "plugin", Required: true, step: stepPlugin, Fix: fixWith("x")},
		{Name: "claude", Required: true, step: stepClaude, Fix: fixWith("x")},
		{Name: "git", step: stepApt, pkg: "git", Fix: fixWith("x")},
		{Name: "python3", Required: true, step: stepApt, pkg: "python3"}, // too old: no fix
		{Name: "ps", OK: true, step: stepApt, pkg: "procps"},
	}
	got := plan(env, checks)
	want := []Step{
		{"apt-update", "apt-get update"},
		{"packages", "apt-get install -y tmux git curl"},
		{"claude", claudeInstall},
		{"marketplace", marketplaceAdd},
		{"handover-plugin", pluginInstall},
	}
	if len(got) != len(want) {
		t.Fatalf("plan = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("step %d = %v, want %v", i, got[i], want[i])
		}
	}
	// No package to install: no apt-get update either.
	for _, s := range plan(env, checks[1:2]) {
		if s.Name == "apt-update" || s.Name == "packages" {
			t.Errorf("plugin only: plan has %v", s)
		}
	}
}

func TestAptUpdate(t *testing.T) {
	t.Parallel()
	if got := aptUpdate(false); got != "sudo apt-get update" {
		t.Errorf("as a user: %q", got)
	}
	if got := aptUpdate(true); got != "apt-get update" {
		t.Errorf("as root: %q", got)
	}
}

func TestAptInstall(t *testing.T) {
	t.Parallel()
	if got := aptInstall(false, "tmux", "libnotify-bin"); got != "sudo apt-get install -y tmux libnotify-bin" {
		t.Errorf("as a user: %q", got)
	}
	if got := aptInstall(true, "tmux"); got != "apt-get install -y tmux" {
		t.Errorf("as root: %q", got)
	}
}
