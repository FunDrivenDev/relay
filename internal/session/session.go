// Package session holds the `relay session` entity: Claude Code under tmux
// and the handover plugin's relay (docs/session.md). run is the relay
// itself, whose environment is the contract with handover.py
// (docs/handover-relay.md).
package session

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/FunDrivenDev/relay/internal/cli"
	"github.com/FunDrivenDev/relay/internal/fail"
	"github.com/FunDrivenDev/relay/internal/run"
)

// Entity is the session entity.
var Entity = &cli.Entity{
	Name:    "session",
	Summary: "Claude Code sessions under tmux and the handover relay",
	Commands: []*cli.Command{
		create,
		list,
		attach,
		runCmd,
	},
}

// PluginID is the handover plugin's key in installed_plugins.json.
const PluginID = "handover@fundrivendev"

// Plugin is the installed handover plugin.
type Plugin struct {
	// Script is the path of its scripts/handover.py.
	Script string
	// Version is the version installed_plugins.json records; empty without one.
	Version string
}

// ConfigDir is Claude Code's config directory: $CLAUDE_CONFIG_DIR, or
// $HOME/.claude; empty when neither is set.
func ConfigDir(env *cli.Env) string {
	if dir := env.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir
	}
	if home := env.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".claude")
	}
	return ""
}

// FindPlugin finds the handover plugin's handover.py: through the plugin's
// installPath in ${CLAUDE_CONFIG_DIR:-$HOME/.claude}/plugins/installed_plugins.json.
// The path changes with every plugin version, so it is looked up on every call.
func FindPlugin(env *cli.Env) (Plugin, error) {
	missing := fail.New(fail.RelayMissing, "%s is not installed", PluginID)
	dir := ConfigDir(env)
	if dir == "" {
		return Plugin{}, missing
	}
	data, err := os.ReadFile(filepath.Join(dir, "plugins", "installed_plugins.json")) //nolint:gosec // Claude Code's own config, where the user points it
	if err != nil {
		return Plugin{}, missing
	}
	var installed struct {
		Plugins map[string][]struct {
			InstallPath string `json:"installPath"`
			Version     string `json:"version"`
		} `json:"plugins"`
	}
	if json.Unmarshal(data, &installed) != nil || len(installed.Plugins[PluginID]) == 0 {
		return Plugin{}, missing
	}
	entry := installed.Plugins[PluginID][0]
	if entry.InstallPath == "" {
		return Plugin{}, missing
	}
	path := filepath.Join(entry.InstallPath, "scripts", "handover.py")
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return Plugin{}, missing
	}
	return Plugin{Script: path, Version: entry.Version}, nil
}

// handoverPy is FindPlugin's handover.py.
func handoverPy(env *cli.Env) (string, error) {
	p, err := FindPlugin(env)
	return p.Script, err
}

// exists reports whether tmux has a session named exactly name.
func exists(ctx context.Context, name string) bool {
	_, err := run.Tool(ctx, "tmux", "has-session", "-t", "="+name)
	return err == nil
}

// self is the command line that runs `relay session run`, for tmux to start.
func self() ([]string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fail.Wrap(fail.Failure, err, "cannot find the relay executable: %v", err)
	}
	return []string{exe, "session", "run"}, nil
}
