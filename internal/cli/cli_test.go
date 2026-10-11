package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/FunDrivenDev/relay/internal/fail"
)

type testParams struct {
	Name  string   `json:"name"`
	Count int      `json:"count"`
	Tags  []string `json:"tags"`
	Force bool     `json:"force"`
	Level string   `json:"level"`
	Color string   `json:"color"`
	Args  []string `json:"args"`
}

var testSpec = Spec{
	Verb: "make", Summary: "makes a thing", Help: "Makes a thing.", Output: `{"name"}`,
	Params: []Param{
		{Name: "name", Help: "the name", Required: true},
		{Name: "count", Short: "c", Help: "how many", Default: 1},
		{Name: "tags", Help: "labels"},
		{Name: "force", Help: "overwrites"},
		{Name: "level", Help: "the level", Enum: []string{"low", "high"}, Default: "low"},
		{Name: "color", Help: "colours", Optional: true, Implicit: "auto"},
		{Name: "args", Help: "passed through"},
	},
	Positional:    "name",
	Rest:          "args",
	StateChanging: true,
}

func newTestCommand(t *testing.T) *Command {
	t.Helper()
	c := New(testSpec, func(_ context.Context, _ *Env, p testParams) (any, error) { return p, nil })
	NewApp("relay", "test", []*Entity{{Name: "thing", Summary: "things", Commands: []*Command{c}}})
	if len(c.problems) > 0 {
		t.Fatalf("problems: %v", c.problems)
	}
	return c
}

func TestParseFlagsAndJSONBuildTheSameParams(t *testing.T) {
	t.Parallel()
	c := newTestCommand(t)
	cases := []struct {
		flags []string
		json  string
	}{
		{[]string{"x"}, `{"name": "x"}`},
		{[]string{"--force", "x", "-c", "3"}, `{"name": "x", "count": 3, "force": true}`},
		{[]string{"--tags=a,b", "--name", "x", "--level", "high"}, `{"name": "x", "tags": ["a", "b"], "level": "high"}`},
		{[]string{"x", "--color"}, `{"name": "x", "color": "auto"}`},
		{[]string{"x", "--dry-run", "--", "--force", "-c"}, `{"name": "x", "dry_run": true, "args": ["--force", "-c"]}`},
		{[]string{"--count=-2", "x"}, `{"name": "x", "count": -2}`},
	}
	for _, tc := range cases {
		fromFlags, err := c.parse(tc.flags, nil)
		if err != nil {
			t.Fatalf("%v: %v", tc.flags, err)
		}
		fromJSON, err := c.parse([]string{"--json=" + tc.json}, nil)
		if err != nil {
			t.Fatalf("%s: %v", tc.json, err)
		}
		if !reflect.DeepEqual(fromFlags.values, fromJSON.values) {
			t.Errorf("%v gives %v, %s gives %v", tc.flags, fromFlags.values, tc.json, fromJSON.values)
		}
		if !reflect.DeepEqual(c.bind(fromFlags.values), c.bind(fromJSON.values)) {
			t.Errorf("%v and %s bind differently", tc.flags, tc.json)
		}
	}
}

func TestParseRejectsWithUsageErrors(t *testing.T) {
	t.Parallel()
	c := newTestCommand(t)
	cases := map[string][]string{
		"--count expects a value":           {"x", "--count", "-1"},
		"--name expects a value":            {"--name"},
		"--force takes no value":            {"x", "--force=true"},
		"repeated option: --force":          {"x", "--force", "--force"},
		"repeated option: --count":          {"x", "-c", "1", "--count=2"},
		"unknown option: --bogus":           {"x", "--bogus"},
		"unknown option: --dry_run":         {"x", "--dry_run"},
		"unknown option: --args":            {"x", "--args", "y"},
		"unknown option: -z":                {"x", "-z"},
		"unknown option: -":                 {"x", "-"},
		"only one positional argument":      {"x", "y"},
		"missing <name>":                    {"--force"},
		`--level: "mid" is not one of`:      {"x", "--level", "mid"},
		`--count expects an integer`:        {"x", "--count", "many"},
		"carries all the inputs":            {"x", `--json={"name": "x"}`},
		"repeated option: --json":           {"--json", `--json={"name": "x"}`},
		"unknown key: bogus":                {`--json={"name": "x", "bogus": 1}`},
		"count: expected an integer":        {`--json={"name": "x", "count": "1"}`},
		"tags: expected an array":           {`--json={"name": "x", "tags": [1]}`},
		"missing key name":                  {`--json={}`},
		"--json expects a JSON object":      {`--json=null`},
		"--json expects a single JSON obj":  {`--json={"name": "x"} 1`},
		`key level: "mid" is not one of`:    {`--json={"name": "x", "level": "mid"}`},
		"no arguments are accepted after -": nil, // checked below on a command without rest
	}
	for want, args := range cases {
		if args == nil {
			continue
		}
		_, err := c.parse(args, nil)
		if err == nil || fail.CodeOf(err) != fail.Usage || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: got %v, want a usage error containing %q", args, err, want)
		}
	}
	noRest := New(Spec{Verb: "v", Summary: "s", Help: "h", Output: "o"}, func(context.Context, *Env, struct{}) (any, error) { return nil, nil })
	if _, err := noRest.parse([]string{"--", "x"}, nil); err == nil || !strings.Contains(err.Error(), "no arguments are accepted after --") {
		t.Errorf("-- on a command without rest: %v", err)
	}
}

func TestParseReadsJSONFromStdin(t *testing.T) {
	t.Parallel()
	c := newTestCommand(t)
	inv, err := c.parse([]string{"--json=-"}, strings.NewReader(`{"name": "x", "force": true}`))
	if err != nil {
		t.Fatal(err)
	}
	if !inv.json || !inv.fromJSON || inv.values["force"] != true || inv.values["count"] != 1 {
		t.Errorf("got %+v", inv)
	}
}

func TestHelpWinsOverEverythingElse(t *testing.T) {
	t.Parallel()
	c := newTestCommand(t)
	inv, err := c.parse([]string{"--bogus", "-h", "--json"}, nil)
	if err != nil || !inv.help || !inv.json {
		t.Errorf("got %+v, %v", inv, err)
	}
	inv, err = c.parse([]string{"x", "--", "--help"}, nil)
	if err != nil || inv.help {
		t.Errorf("--help after -- is passed through: got %+v, %v", inv, err)
	}
}

func TestFlagFormRoundTrips(t *testing.T) {
	t.Parallel()
	c := newTestCommand(t)
	for _, input := range []string{
		`{"name": "a thing", "force": true, "tags": ["x", "y"]}`,
		`{"name": "-dash", "count": -3, "color": "never", "level": "high"}`,
		`{"name": "it's", "args": ["--x", "y z"], "dry_run": true}`,
	} {
		inv, err := c.parse([]string{"--json=" + input}, nil)
		if err != nil {
			t.Fatal(err)
		}
		line := c.FlagForm(inv.values)
		args := shellSplit(t, strings.TrimPrefix(line, "relay thing make"))
		back, err := c.parse(args, nil)
		if err != nil {
			t.Fatalf("%s → %s: %v", input, line, err)
		}
		if !reflect.DeepEqual(back.values, inv.values) {
			t.Errorf("%s → %s → %v", input, line, back.values)
		}
	}
}

// shellSplit splits a line written by quote: words, single-quoted strings.
func shellSplit(t *testing.T, line string) []string {
	t.Helper()
	var out []string
	var cur strings.Builder
	inWord, quoted := false, false
	for i := 0; i < len(line); i++ {
		ch := line[i]
		switch {
		case quoted && ch == '\'':
			quoted = false
		case quoted:
			cur.WriteByte(ch)
		case ch == '\'':
			quoted, inWord = true, true
		case ch == '\\' && i+1 < len(line):
			i++
			cur.WriteByte(line[i])
		case ch == ' ':
			if inWord {
				out = append(out, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteByte(ch)
			inWord = true
		}
	}
	if inWord {
		out = append(out, cur.String())
	}
	return out
}

func TestSpecProblems(t *testing.T) {
	t.Parallel()
	type params struct {
		A string  `json:"a"`
		B float64 `json:"b"`
		C string
		D []string `json:"d"`
	}
	c := New(Spec{
		Verb: "Bad_Verb",
		Params: []Param{
			{Name: "a", Help: "a", Default: 3, Required: true},
			{Name: "b", Help: "b"},
			{Name: "zz", Help: "zz"},
			{Name: "d", Short: "h", Enum: []string{"x"}, Optional: true},
		},
		Positional: "nope",
		Rest:       "a",
		DryRunJSON: true,
	}, func(context.Context, *Env, params) (any, error) { return nil, nil })
	want := []string{
		`verb "Bad_Verb" is not kebab-case`, "no summary", "no help", "no output description",
		"field C has no json tag", `param "a": default 3 is not a string`, `param "a" is required and has a default`,
		`param "b": field type float64`, `param "zz" has no field`, `param "d" has no help`,
		`param "d": short "h"`, `param "d": only a string param can have an optional value`,
		`positional "nope" is not a param`, `rest "a" is not an array param`,
		"dry_run_json needs an interactive, state-changing command",
	}
	got := strings.Join(c.problems, "\n")
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("missing problem %q in:\n%s", w, got)
		}
	}
}

func TestVerbGroups(t *testing.T) {
	t.Parallel()
	run := func(context.Context, *Env, struct{}) (any, error) { return nil, nil }
	cmd := func(verb string) *Command {
		return New(Spec{Verb: verb, Summary: "s", Help: "h", Output: "o"}, run)
	}
	for _, verb := range []string{"a b c", "a  b", "a b-", " a"} {
		if len(cmd(verb).problems) == 0 {
			t.Errorf("verb %q accepted", verb)
		}
	}
	e := &Entity{Name: "e", Summary: "s", Commands: []*Command{cmd("keys"), cmd("keys import"), cmd("other verify")}}
	got := strings.Join(e.Problems(), "\n")
	if !strings.Contains(got, "keys is both a verb and a group") || strings.Contains(got, "other") {
		t.Errorf("problems: %s", got)
	}
}

func TestMainReportsErrorsAsEnvelopes(t *testing.T) {
	t.Parallel()
	c := New(Spec{Verb: "go", Summary: "s", Help: "h", Output: "o"}, func(context.Context, *Env, struct{}) (any, error) {
		return map[string]int{"n": 1}, fail.New(fail.NotFound, "no such thing")
	})
	app := NewApp("relay", "test", []*Entity{{Name: "thing", Summary: "things", Commands: []*Command{c}}})
	var stdout, stderr bytes.Buffer
	status := app.Main(context.Background(), []string{"thing", "go", "--json"}, System{Stdout: &stdout, Stderr: &stderr})
	if status != 1 {
		t.Errorf("status %d, want 1", status)
	}
	var envelope struct {
		Error struct{ Code, Message string }
	}
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil || envelope.Error.Code != "not_found" || envelope.Error.Message != "no such thing" {
		t.Errorf("stderr %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"n": 1`) {
		t.Errorf("stdout %q", stdout.String())
	}
}
