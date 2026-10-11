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

// pluginID is the handover plugin's key in installed_plugins.json.
const pluginID = "handover@fundrivendev"

// handoverPy finds the handover plugin's handover.py: through the plugin's
// installPath in ${CLAUDE_CONFIG_DIR:-$HOME/.claude}/plugins/installed_plugins.json.
// The path changes with every plugin version, so it is looked up on every call.
func handoverPy(env *cli.Env) (string, error) {
	missing := fail.New(fail.RelayMissing, "%s is not installed", pluginID)
	dir := env.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		home := env.Getenv("HOME")
		if home == "" {
			return "", missing
		}
		dir = filepath.Join(home, ".claude")
	}
	data, err := os.ReadFile(filepath.Join(dir, "plugins", "installed_plugins.json")) //nolint:gosec // Claude Code's own config, where the user points it
	if err != nil {
		return "", missing
	}
	var installed struct {
		Plugins map[string][]struct {
			InstallPath string `json:"installPath"`
		} `json:"plugins"`
	}
	if json.Unmarshal(data, &installed) != nil || len(installed.Plugins[pluginID]) == 0 {
		return "", missing
	}
	root := installed.Plugins[pluginID][0].InstallPath
	if root == "" {
		return "", missing
	}
	path := filepath.Join(root, "scripts", "handover.py")
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return "", missing
	}
	return path, nil
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
