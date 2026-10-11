// Package doctor holds the `relay doctor` entity: what relay needs on this
// machine, checked (doctor check) and installed (doctor fix), for Linux and
// Ubuntu's packages first (docs/doctor.md).
package doctor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/FunDrivenDev/relay/internal/cli"
	"github.com/FunDrivenDev/relay/internal/fail"
	"github.com/FunDrivenDev/relay/internal/notifier"
	"github.com/FunDrivenDev/relay/internal/run"
	"github.com/FunDrivenDev/relay/internal/session"
	"github.com/FunDrivenDev/relay/internal/ui"
)

// Entity is the doctor entity.
var Entity = &cli.Entity{
	Name:    "doctor",
	Summary: "checks and installs what relay needs on this machine",
	Commands: []*cli.Command{
		check,
		fix,
	},
}

// The floors relay holds the tools to (docs/doctor.md says why).
var (
	// tmuxFloor: new-session -e, which the relay uses, came in tmux 3.2.
	tmuxFloor = version{3, 2}
	// pythonFloor: handover.py uses str.removeprefix, new in Python 3.9.
	pythonFloor = version{3, 9}
)

// The fixes, as people type them on Ubuntu.
const (
	// claudeInstall is Claude Code's official installer; it puts claude in
	// ~/.local/bin.
	claudeInstall = "curl -fsSL https://claude.ai/install.sh | bash"
	// marketplace is the marketplace the handover plugin comes from.
	marketplace     = "fundrivendev"
	marketplaceRepo = "FunDrivenDev/claude-plugins"
	marketplaceAdd  = "claude plugin marketplace add " + marketplaceRepo
	pluginInstall   = "claude plugin install " + session.PluginID
)

// Check is one requirement checked, with the command that fixes it.
type Check struct {
	Name     string `json:"name"`
	OK       bool   `json:"ok"`
	Required bool   `json:"required"`
	// Version is what was found: the tool's version, the distribution, the
	// plugin's version; empty when nothing was.
	Version string  `json:"version"`
	Message string  `json:"message"`
	Fix     *string `json:"fix"`

	// pkg is the apt package that fixes it, merged into one apt-get line;
	// install is set when installing it would.
	pkg     string
	install bool
	// step orders the fixes: apt first, then claude, then the plugin.
	step int
}

// The steps of a fix, in the order they run.
const (
	stepNone = iota
	stepApt
	stepClaude
	stepPlugin
)

// Step is one command of the fix plan.
type Step struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

// Report is what doctor check prints.
type Report struct {
	OK     bool    `json:"ok"`
	OS     string  `json:"os"`
	Distro string  `json:"distro"`
	Checks []Check `json:"checks"`
	// Fixes is the plan doctor fix runs, in order.
	Fixes []Step `json:"fixes"`
}

// Human prints the checklist: ✓ met, ✗ a required check failed, ! an
// optional one; each unmet check followed by its fix.
func (r Report) Human(t ui.Theme) string {
	var b strings.Builder
	b.WriteString(checklist(t, r.Checks))
	switch {
	case len(r.Fixes) > 0:
		fmt.Fprintf(&b, "\n%s %s\n", t.Muted.Render("To install what is missing:"), t.Command.Render("relay doctor fix"))
	case r.OK:
		fmt.Fprintf(&b, "\n%s\n", t.OK.Render("relay is ready."))
	}
	return b.String()
}

// checklist renders one line per check, then its fix under it.
func checklist(t ui.Theme, checks []Check) string {
	width := 0
	for _, c := range checks {
		width = max(width, len(c.Name))
	}
	var b strings.Builder
	for _, c := range checks {
		mark := t.OK.Render("✓")
		switch {
		case !c.OK && c.Required:
			mark = t.Error.Render("✗")
		case !c.OK:
			mark = t.Warn.Render("!")
		}
		name := c.Name + strings.Repeat(" ", width-len(c.Name))
		fmt.Fprintf(&b, "%s %s  %s\n", mark, t.Title.Render(name), c.Message)
		if !c.OK && c.Fix != nil {
			for line := range strings.Lines(*c.Fix) {
				fmt.Fprintf(&b, "  %s  %s\n", strings.Repeat(" ", width), t.Command.Render(strings.TrimRight(line, "\n")))
			}
		}
	}
	return b.String()
}

type checkParams struct{}

var check = cli.New(cli.Spec{
	Verb:    "check",
	Summary: "checks what relay needs: tmux, claude, python3, ps, the handover plugin",
	Help: `Checks, without changing anything, what relay needs on this machine:

  os               Linux (a warning elsewhere), the distribution from /etc/os-release
  tmux             on PATH, 3.2 or later (new-session -e)
  claude           Claude Code on PATH
  python3          on PATH, 3.9 or later (the plugin's handover.py)
  ps               procps' ps, which handover.py walks (ps -o ppid= -p)
  git              optional: handover.py reads the repo for context
  notify-send      optional: desktop alerts of AFK runs on a Linux desktop
                   (libnotify-bin); elsewhere notify_command is the way
  handover-plugin  handover@fundrivendev installed in Claude Code
  notifications    optional: the plugin's alerts reach you (relay notifier check)

Each unmet check comes with the command that fixes it, for Ubuntu; relay
doctor fix runs them. Exit 1 when a required check fails (code
requirements_missing): under --json the report stays on stdout and the error
goes to stderr.`,
	Output: `{"ok", "os", "distro", "checks": [{"name", "ok", "required", "version", "message", "fix"}], "fixes": [{"name", "command"}]}`,
}, func(ctx context.Context, env *cli.Env, _ checkParams) (any, error) {
	r := inspect(ctx, env)
	return r, r.err(env)
})

// err is the error a report with a failing required check ends on: quiet
// without --json, where the checklist already says it.
func (r Report) err(env *cli.Env) error {
	if r.OK {
		return nil
	}
	var missing []string
	for _, c := range r.Checks {
		if c.Required && !c.OK {
			missing = append(missing, c.Name)
		}
	}
	return &fail.Error{
		Code: fail.RequirementsMissing, Silent: !env.JSON,
		Message: "missing requirements: " + strings.Join(missing, ", ") + " (relay doctor fix)",
	}
}

// inspect runs every check and plans the fixes.
func inspect(ctx context.Context, env *cli.Env) Report {
	goos := env.Getenv("RELAY_GOOS")
	if goos == "" {
		goos = runtime.GOOS
	}
	osCheck, distro := checkOS(env, goos)
	// The plugin's view of the notifications, which the notify-send check
	// follows: relay does not detect the desktop itself.
	alerts, _, alertsErr := notifier.Inspect(ctx, env, false)
	checks := []Check{
		osCheck,
		checkTmux(ctx),
		checkClaude(ctx, env),
		checkPython(ctx),
		checkPS(ctx),
		checkGit(ctx),
		checkNotify(ctx, alerts, alertsErr),
		checkPlugin(env),
		checkNotifications(alerts, alertsErr),
	}
	r := Report{OK: true, OS: goos, Distro: distro, Checks: checks}
	root := asRoot(env)
	for i, c := range r.Checks {
		// apt-get is Ubuntu's: elsewhere the package manager is the user's.
		if c.install && goos == "linux" {
			r.Checks[i].Fix = fixWith(aptInstall(root, c.pkg))
		}
		if c.Required && !c.OK {
			r.OK = false
		}
	}
	r.Fixes = plan(env, r.Checks)
	return r
}

// plan merges the fixes of the unmet checks into the commands to run: one
// apt-get line for every missing package, then Claude Code's installer, then
// the plugin.
func plan(env *cli.Env, checks []Check) []Step {
	root := asRoot(env)
	var pkgs []string
	claude, plugin := false, false
	for _, c := range checks {
		if c.OK || c.Fix == nil {
			continue
		}
		switch c.step {
		case stepApt:
			pkgs = append(pkgs, c.pkg)
		case stepClaude:
			claude = true
		case stepPlugin:
			plugin = true
		}
	}
	// The installer is piped from curl.
	if claude && !run.Has("curl") {
		pkgs = append(pkgs, "curl")
	}
	steps := []Step{}
	if len(pkgs) > 0 {
		steps = append(steps, Step{Name: "packages", Command: aptInstall(root, pkgs...)})
	}
	if claude {
		steps = append(steps, Step{Name: "claude", Command: claudeInstall})
	}
	if plugin {
		if !marketplaceKnown(env) {
			steps = append(steps, Step{Name: "marketplace", Command: marketplaceAdd})
		}
		steps = append(steps, Step{Name: "handover-plugin", Command: pluginInstall})
	}
	return steps
}

// asRoot reports whether relay runs as root, where apt-get needs no sudo:
// $RELAY_EUID stands for the effective user id when set (the tests).
func asRoot(env *cli.Env) bool {
	if v := env.Getenv("RELAY_EUID"); v != "" {
		return v == "0"
	}
	return os.Geteuid() == 0
}

// aptInstall is the apt-get line that installs pkgs, through sudo unless
// relay runs as root.
func aptInstall(root bool, pkgs ...string) string {
	line := "apt-get install -y " + strings.Join(pkgs, " ")
	if !root {
		line = "sudo " + line
	}
	return line
}

func fixWith(command string) *string { return &command }

// osRelease reads the distribution's name from os-release
// ($RELAY_OS_RELEASE, /etc/os-release by default): PRETTY_NAME, else NAME
// and VERSION.
func osRelease(env *cli.Env) string {
	path := env.Getenv("RELAY_OS_RELEASE")
	if path == "" {
		path = "/etc/os-release"
	}
	f, err := os.Open(path) //nolint:gosec // the system's os-release, or the one a test points to
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	vars := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok || strings.HasPrefix(k, "#") {
			continue
		}
		if u, err := strconv.Unquote(v); err == nil {
			v = u
		} else {
			v = strings.Trim(v, `'"`)
		}
		vars[k] = v
	}
	if vars["PRETTY_NAME"] != "" {
		return vars["PRETTY_NAME"]
	}
	return strings.TrimSpace(vars["NAME"] + " " + vars["VERSION"])
}

func checkOS(env *cli.Env, goos string) (Check, string) {
	c := Check{Name: "os"}
	if goos != "linux" {
		c.Message = goos + ": relay is made for Linux; install the tools below with your package manager"
		return c, ""
	}
	distro := osRelease(env)
	c.OK, c.Version = true, distro
	c.Message = "linux"
	if distro != "" {
		c.Message += ", " + distro
	}
	return c, distro
}

// version is a major.minor version.
type version struct{ major, minor int }

func (v version) String() string { return fmt.Sprintf("%d.%d", v.major, v.minor) }

func (v version) less(w version) bool {
	return v.major < w.major || (v.major == w.major && v.minor < w.minor)
}

var versionRE = regexp.MustCompile(`(\d+)\.(\d+)`)

// parseVersion finds the first major.minor in s.
func parseVersion(s string) (version, bool) {
	m := versionRE.FindStringSubmatch(s)
	if m == nil {
		return version{}, false
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	return version{major, minor}, true
}

// tool runs name with args and returns its output, stdout then stderr,
// trimmed; found is false when name is not on PATH.
func tool(ctx context.Context, name string, args ...string) (out string, found bool, err error) {
	if !run.Has(name) {
		return "", false, nil
	}
	res, err := run.Tool(ctx, name, args...)
	return strings.TrimSpace(res.Stdout + "\n" + res.Stderr), true, err
}

// toolVersion is the version token of a --version line: the first word
// holding a digit and a dot ("tmux 3.4" → "3.4", "Python 3.12.3" → "3.12.3").
func toolVersion(line string) string {
	line, _, _ = strings.Cut(line, "\n")
	for _, w := range strings.Fields(line) {
		if versionRE.MatchString(w) {
			return strings.TrimPrefix(w, "next-")
		}
	}
	return ""
}

func checkTmux(ctx context.Context) Check {
	c := Check{Name: "tmux", Required: true, pkg: "tmux", step: stepApt}
	out, found, err := tool(ctx, "tmux", "-V")
	switch {
	case !found:
		c.Message = "not found on PATH"
		c.install = true
		return c
	case err != nil:
		c.Message = "tmux -V failed: " + err.Error()
		return c
	}
	c.Version = toolVersion(out)
	v, ok := parseVersion(out)
	if ok && v.less(tmuxFloor) {
		c.Message = fmt.Sprintf("tmux %s is older than %s (new-session -e): Ubuntu 22.04 or later ships a recent one", c.Version, tmuxFloor)
		return c
	}
	c.OK = true
	c.Message = "tmux " + c.Version
	if !ok {
		c.Message = out + " (version unknown, taken as recent)"
	}
	return c
}

func checkClaude(ctx context.Context, env *cli.Env) Check {
	c := Check{Name: "claude", Required: true, step: stepClaude}
	out, found, err := tool(ctx, "claude", "--version")
	if !found {
		// Where Claude Code's installer puts it: installed, but off PATH.
		if home := env.Getenv("HOME"); home != "" {
			if info, err := os.Stat(filepath.Join(home, ".local", "bin", "claude")); err == nil && !info.IsDir() {
				c.Message = `~/.local/bin/claude is not on PATH: add it to your shell's profile (export PATH="$HOME/.local/bin:$PATH")`
				return c
			}
		}
		c.Message = "Claude Code not found on PATH"
		c.Fix = fixWith(claudeInstall)
		return c
	}
	c.OK = true
	c.Version = toolVersion(out)
	c.Message = "Claude Code " + c.Version
	if err != nil || c.Version == "" {
		c.Message = "Claude Code on PATH (claude --version did not answer with a version)"
	}
	return c
}

func checkPython(ctx context.Context) Check {
	c := Check{Name: "python3", Required: true, pkg: "python3", step: stepApt}
	out, found, err := tool(ctx, "python3", "--version")
	switch {
	case !found:
		c.Message = "not found on PATH"
		c.install = true
		return c
	case err != nil:
		c.Message = "python3 --version failed: " + err.Error()
		return c
	}
	c.Version = toolVersion(out)
	v, ok := parseVersion(out)
	if !ok || v.less(pythonFloor) {
		c.Message = fmt.Sprintf("Python %s is older than %s, which handover.py needs", c.Version, pythonFloor)
		return c
	}
	c.OK = true
	c.Message = "Python " + c.Version
	return c
}

func checkPS(ctx context.Context) Check {
	c := Check{Name: "ps", Required: true, pkg: "procps", step: stepApt}
	if !run.Has("ps") {
		c.Message = "not found on PATH"
		c.install = true
		return c
	}
	// handover.py walks the process tree with these options.
	if _, err := run.Tool(ctx, "ps", "-o", "ppid=", "-p", "1"); err != nil {
		c.Message = "ps does not take -o ppid= -p (procps' ps is needed): " + err.Error()
		c.install = true
		return c
	}
	c.OK = true
	c.Message = "ps"
	if out, _, err := tool(ctx, "ps", "--version"); err == nil {
		c.Version = toolVersion(out)
		c.Message = strings.TrimSpace("ps " + c.Version)
	}
	return c
}

func checkGit(ctx context.Context) Check {
	c := Check{Name: "git", pkg: "git", step: stepApt}
	out, found, _ := tool(ctx, "git", "--version")
	if !found {
		c.Message = "not found (optional: handover.py reads the repo for context)"
		c.install = true
		return c
	}
	c.OK = true
	c.Version = toolVersion(out)
	c.Message = strings.TrimSpace("git " + c.Version)
	return c
}

// checkNotify checks notify-send, which only a Linux desktop uses: where the
// plugin finds none (headless, SSH, WSL, macOS), alerts need notify_command.
func checkNotify(ctx context.Context, alerts notifier.Report, alertsErr error) Check {
	c := Check{Name: "notify-send", pkg: "libnotify-bin", step: stepApt}
	out, found, _ := tool(ctx, "notify-send", "--version")
	if !found {
		if e := alerts.Environment; alertsErr == nil && (e.OS != "linux" || !e.Desktop) {
			c.OK = true
			c.Message = "not used here (" + e.Why + "): set notify_command (Slack, ntfy…) to be reached, see notifications"
			return c
		}
		c.Message = "not found (optional: desktop alerts of AFK runs; without it, the terminal bell)"
		c.install = true
		return c
	}
	c.OK = true
	c.Version = toolVersion(out)
	c.Message = strings.TrimSpace("notify-send " + c.Version)
	return c
}

// checkNotifications reports, from the plugin, whether its alerts reach you
// here: optional, skipped without the plugin or python3.
func checkNotifications(alerts notifier.Report, err error) Check {
	c := Check{Name: "notifications"}
	switch {
	case fail.CodeOf(err) == fail.RelayMissing:
		c.Message = "skipped: the handover plugin is not installed"
		return c
	case fail.CodeOf(err) == fail.ToolMissing:
		c.Message = "skipped: python3 is missing"
		return c
	case err != nil:
		c.Message = "skipped: " + err.Error()
		return c
	}
	c.OK = alerts.ReachesYou
	c.Message = alerts.Environment.Where() + ": " + strings.Join(alerts.Adapters, ", ")
	if !c.OK || len(alerts.Hints) > 0 {
		c.Fix = fixWith(strings.Join(alerts.Advice(), "\n"))
	}
	return c
}

func checkPlugin(env *cli.Env) Check {
	c := Check{Name: "handover-plugin", Required: true, step: stepPlugin}
	p, err := session.FindPlugin(env)
	if err != nil {
		c.Message = session.PluginID + " is not installed in Claude Code"
		cmd := pluginInstall
		if !marketplaceKnown(env) {
			cmd = marketplaceAdd + " && " + pluginInstall
		}
		c.Fix = fixWith(cmd)
		return c
	}
	c.OK = true
	c.Version = p.Version
	c.Message = strings.TrimSpace(session.PluginID + " " + p.Version)
	return c
}

// marketplaceKnown reports whether Claude Code knows the fundrivendev
// marketplace (plugins/known_marketplaces.json).
func marketplaceKnown(env *cli.Env) bool {
	dir := session.ConfigDir(env)
	if dir == "" {
		return false
	}
	data, err := os.ReadFile(filepath.Join(dir, "plugins", "known_marketplaces.json")) //nolint:gosec // Claude Code's own config
	if err != nil {
		return false
	}
	var known map[string]json.RawMessage
	if json.Unmarshal(data, &known) != nil {
		return false
	}
	_, ok := known[marketplace]
	return ok
}
