package session

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/FunDrivenDev/relay/internal/run"
)

// LoginHint is what to do when Claude Code is not logged in: sign in once
// interactively, or, headless, give it a token or an API key in the
// environment.
const LoginHint = "run claude once to sign in (or claude auth login); headless: " +
	"claude setup-token, then export CLAUDE_CODE_OAUTH_TOKEN, or set ANTHROPIC_API_KEY"

// Login is Claude Code's own account of its authentication, as
// `claude auth status --json` prints it. It counts ANTHROPIC_API_KEY,
// CLAUDE_CODE_OAUTH_TOKEN and the cloud providers' variables (Bedrock,
// Vertex…) as well as a stored login, and never prints a secret.
type Login struct {
	// Known is false when claude is missing or did not answer with a status
	// (a Claude Code without `auth status`).
	Known      bool
	LoggedIn   bool
	AuthMethod string // "claude.ai", "oauth_token", "api_key", "third_party"…
	Provider   string // "firstParty", "bedrock", "vertex"…
}

// Describe is the login in a few words: "oauth_token" or
// "third_party, bedrock".
func (l Login) Describe() string {
	parts := []string{}
	if l.AuthMethod != "" {
		parts = append(parts, l.AuthMethod)
	}
	if l.Provider != "" && l.Provider != "firstParty" {
		parts = append(parts, l.Provider)
	}
	return strings.Join(parts, ", ")
}

// CheckLogin asks Claude Code whether it is logged in. It exits 1 when it is
// not, with the same JSON on stdout: the exit status is not the answer.
func CheckLogin(ctx context.Context) Login {
	if !run.Has("claude") {
		return Login{}
	}
	res, _ := run.Tool(ctx, "claude", "auth", "status", "--json")
	var status struct {
		LoggedIn    *bool  `json:"loggedIn"`
		AuthMethod  string `json:"authMethod"`
		APIProvider string `json:"apiProvider"`
	}
	if json.Unmarshal([]byte(res.Stdout), &status) != nil || status.LoggedIn == nil {
		return Login{}
	}
	return Login{Known: true, LoggedIn: *status.LoggedIn, AuthMethod: status.AuthMethod, Provider: status.APIProvider}
}
