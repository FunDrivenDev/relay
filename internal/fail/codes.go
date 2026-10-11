package fail

// Code is a stable error code: agents branch on it, so a code is never renamed
// or reused for another meaning. Every code is declared here, with a line in
// docs/errors.md (a test checks both lists match).
type Code string

// The codes. Keep them sorted within each group; an entity adds its own codes
// in its group, one line each.
const (
	// Usage: the command line or the JSON input is wrong (exit status 2).
	Usage Code = "usage"

	// Failure: an error without a more precise code.
	Failure Code = "failure"
	// Exists: what the command would create already exists.
	Exists Code = "exists"
	// Interrupted: the command was cancelled (Ctrl-C, SIGTERM).
	Interrupted Code = "interrupted"
	// NotFound: what the command acts on does not exist.
	NotFound Code = "not_found"
	// ToolFailed: an external tool (tmux, python3, claude…) exited with an error.
	ToolFailed Code = "tool_failed"
	// ToolMissing: an external tool is not on PATH.
	ToolMissing Code = "tool_missing"
	// TTYRequired: an interactive command was run under --json, or without a
	// terminal to ask on.
	TTYRequired Code = "tty_required"

	// session: Claude Code under tmux and the handover relay.
	// RelayMissing: the handover plugin is not installed (handover.py not found
	// through its installPath in installed_plugins.json).
	RelayMissing Code = "relay_missing"

	// doctor: what relay needs on this machine.
	// Declined: the person answered no to the confirmation; nothing ran.
	Declined Code = "declined"
	// FixFailed: a fix command exited with an error; the ones after it did not run.
	FixFailed Code = "fix_failed"
	// RequirementsMissing: a required check failed; the report lists the fixes.
	RequirementsMissing Code = "requirements_missing"

	// notifier: the handover plugin's notifications.
	// NotSent: the test alert got through no notifier, not even the bell.
	NotSent Code = "not_sent"
)

// Codes lists every code, for the docs check.
var Codes = []Code{
	Usage,
	Failure,
	Exists,
	Interrupted,
	NotFound,
	ToolFailed,
	ToolMissing,
	TTYRequired,
	RelayMissing,
	Declined,
	FixFailed,
	RequirementsMissing,
	NotSent,
}
