package run

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/FunDrivenDev/relay/internal/fail"
)

func TestToolCapturesOutput(t *testing.T) {
	t.Parallel()
	res, err := Opts{Stdin: strings.NewReader("in")}.Tool(context.Background(), "sh", "-c", "cat; echo out; echo err >&2")
	if err != nil {
		t.Fatal(err)
	}
	if res.Stdout != "inout\n" || res.Stderr != "err\n" {
		t.Errorf("got %+v", res)
	}
}

func TestAttachWritesToTheGivenWriters(t *testing.T) {
	t.Parallel()
	var out, errOut bytes.Buffer
	o := Opts{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
	err := o.Attach(context.Background(), "sh", "-c", "echo out; echo err >&2; exit 2")
	var e *Error
	if !errors.As(err, &e) || e.ExitCode != 2 {
		t.Fatalf("got %v", err)
	}
	if out.String() != "out\n" || errOut.String() != "err\n" {
		t.Errorf("got %q, %q", out.String(), errOut.String())
	}
}

func TestToolErrorsCarryTheirCode(t *testing.T) {
	t.Parallel()
	res, err := Tool(context.Background(), "sh", "-c", "echo partial; echo 'bad thing\nmore' >&2; exit 3")
	var e *Error
	if !errors.As(err, &e) || e.ExitCode != 3 || fail.CodeOf(err) != fail.ToolFailed {
		t.Fatalf("got %v", err)
	}
	if res.Stdout != "partial\n" || !strings.HasSuffix(err.Error(), "exited 3: bad thing") {
		t.Errorf("got %q, %v", res.Stdout, err)
	}
	_, err = Tool(context.Background(), "relay-no-such-tool")
	if fail.CodeOf(err) != fail.ToolMissing || err.Error() != "relay-no-such-tool not found on PATH" {
		t.Errorf("got %v", err)
	}
	if Has("relay-no-such-tool") || !Has("sh") {
		t.Error("Has")
	}
}

func TestMergeEnvLastValueWins(t *testing.T) {
	t.Parallel()
	got := mergeEnv([]string{"A=1", "B=2"}, []string{"B=", "C=3", "A=4"})
	want := []string{"A=4", "B=", "C=3"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}
