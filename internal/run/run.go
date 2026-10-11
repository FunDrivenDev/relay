// Package run runs the external tools relay drives (tmux, claude, python3,
// sudo…). A tool is looked up on PATH at each call, so the
// functional tests replace it with a stub placed first on PATH.
package run

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"

	"github.com/FunDrivenDev/relay/internal/fail"
)

// Result is what a tool printed.
type Result struct {
	Stdout string
	Stderr string
}

// Error is a tool that could not run (code tool_missing) or exited with a
// non-zero status (code tool_failed).
type Error struct {
	Tool     string
	Args     []string
	ExitCode int // -1 when the tool did not start
	Stdout   string
	Stderr   string
	Err      error
}

func (e *Error) Error() string {
	if e.ExitCode < 0 {
		if errors.Is(e.Err, exec.ErrNotFound) {
			return e.Tool + " not found on PATH"
		}
		return fmt.Sprintf("%s: %v", e.Tool, e.Err)
	}
	msg := fmt.Sprintf("%s exited %d", e.command(), e.ExitCode)
	if line := firstLine(e.Stderr); line != "" {
		msg += ": " + line
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Err }

// FailCode maps the error to tool_missing or tool_failed.
func (e *Error) FailCode() fail.Code {
	if errors.Is(e.Err, exec.ErrNotFound) {
		return fail.ToolMissing
	}
	return fail.ToolFailed
}

func (e *Error) command() string {
	return strings.TrimSpace(e.Tool + " " + strings.Join(e.Args, " "))
}

// Opts tunes a call to Tool, Attach or Exec; its zero value runs in the
// current directory with the process environment.
type Opts struct {
	// Dir is the working directory; the current one when empty.
	Dir string
	// Env are KEY=value pairs added to the process environment; a key set
	// again replaces its value (the last one wins).
	Env []string
	// Stdin is the input: none for Tool, the terminal for Attach, when nil.
	// Exec ignores it: the new program keeps the terminal.
	Stdin io.Reader
	// Stdout and Stderr receive the output of Attach; the process's own
	// when nil. Tool captures the output and Exec keeps the terminal: both
	// ignore them.
	Stdout io.Writer
	Stderr io.Writer
}

// Tool runs name with args and captures its output.
func Tool(ctx context.Context, name string, args ...string) (Result, error) {
	return Opts{}.Tool(ctx, name, args...)
}

// Tool runs name with args under o and captures its output. A non-zero exit
// returns the output along with an *Error.
func (o Opts) Tool(ctx context.Context, name string, args ...string) (Result, error) {
	cmd, err := o.command(ctx, name, args)
	if err != nil {
		return Result{}, err
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	res := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if err != nil {
		return res, failure(ctx, name, args, err, res)
	}
	return res, nil
}

// Attach runs name with args on the terminal: it inherits stdin, stdout and
// stderr (an editor, tmux attach, an interactive prompt).
func Attach(ctx context.Context, name string, args ...string) error {
	return Opts{}.Attach(ctx, name, args...)
}

// Attach runs name with args under o, on the terminal, or on o.Stdout and
// o.Stderr when set.
func (o Opts) Attach(ctx context.Context, name string, args ...string) error {
	cmd, err := o.command(ctx, name, args)
	if err != nil {
		return err
	}
	if o.Stdin == nil {
		cmd.Stdin = os.Stdin
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if o.Stdout != nil {
		cmd.Stdout = o.Stdout
	}
	if o.Stderr != nil {
		cmd.Stderr = o.Stderr
	}
	if err := cmd.Run(); err != nil {
		return failure(ctx, name, args, err, Result{})
	}
	return nil
}

// Exec replaces the relay process with name and args (tmux attach as the
// last step of a command). It returns only on failure.
func Exec(name string, args ...string) error {
	return Opts{}.Exec(name, args...)
}

// Exec replaces the relay process with name and args under o (o.Dir
// becomes the process's working directory). It returns only on failure.
func (o Opts) Exec(name string, args ...string) error {
	path, err := lookPath(name, args)
	if err != nil {
		return err
	}
	if o.Dir != "" {
		if err := os.Chdir(o.Dir); err != nil {
			return &Error{Tool: name, Args: args, ExitCode: -1, Err: err}
		}
	}
	err = syscall.Exec(path, append([]string{name}, args...), o.environ()) //nolint:gosec // the tool and its arguments are the caller's to choose
	return &Error{Tool: name, Args: args, ExitCode: -1, Err: err}
}

// environ is the environment a program run under o gets: the process's,
// with o.Env added.
func (o Opts) environ() []string {
	return mergeEnv(os.Environ(), o.Env)
}

// mergeEnv returns base with extra added, as KEY=value pairs: a key set in
// extra replaces its value in base, and a key set twice keeps its last value,
// as os/exec does (syscall.Exec does not, and a program's getenv would read
// the first).
func mergeEnv(base, extra []string) []string {
	out := make([]string, 0, len(base)+len(extra))
	at := map[string]int{}
	for _, kv := range append(slices.Clip(base), extra...) {
		k, _, _ := strings.Cut(kv, "=")
		if i, ok := at[k]; ok {
			out[i] = kv
			continue
		}
		at[k] = len(out)
		out = append(out, kv)
	}
	return out
}

// Has reports whether name is on PATH.
func Has(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func (o Opts) command(ctx context.Context, name string, args []string) (*exec.Cmd, error) {
	path, err := lookPath(name, args)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, path, args...) //nolint:gosec // the tool and its arguments are the caller's to choose
	cmd.Args[0] = name
	cmd.Dir = o.Dir
	if len(o.Env) > 0 {
		cmd.Env = o.environ()
	}
	cmd.Stdin = o.Stdin
	return cmd, nil
}

// lookPath finds name on PATH, or returns the tool_missing error.
func lookPath(name string, args []string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", &Error{Tool: name, Args: args, ExitCode: -1, Err: err}
	}
	return path, nil
}

func failure(ctx context.Context, name string, args []string, err error, res Result) error {
	if ctx.Err() != nil {
		return fail.Wrap(fail.Interrupted, ctx.Err(), "%s interrupted", name)
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return &Error{Tool: name, Args: args, ExitCode: exit.ExitCode(), Stdout: res.Stdout, Stderr: res.Stderr, Err: err}
	}
	return &Error{Tool: name, Args: args, ExitCode: -1, Err: err}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}
