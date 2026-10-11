package fail

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
)

// TestCodesAreDocumented checks that docs/errors.md lists every code, and
// only those.
func TestCodesAreDocumented(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "errors.md"))
	if err != nil {
		t.Fatal(err)
	}
	documented := map[Code]bool{}
	for _, m := range regexp.MustCompile("(?m)^\\| `([a-z_]+)` \\|").FindAllSubmatch(data, -1) {
		documented[Code(m[1])] = true
	}
	for _, c := range Codes {
		if !documented[c] {
			t.Errorf("code %s is not in docs/errors.md", c)
		}
	}
	for c := range documented {
		if !slices.Contains(Codes, c) {
			t.Errorf("docs/errors.md lists %s, which fail.Codes lacks", c)
		}
	}
}

func TestExitStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		want int
	}{
		{nil, 0},
		{Usagef("bad"), 2},
		{Quiet(Declined), 1},
		{New(NotFound, "x"), 1},
		{os.ErrNotExist, 1},
	}
	for _, tc := range cases {
		if got := ExitStatus(tc.err); got != tc.want {
			t.Errorf("ExitStatus(%v) = %d, want %d", tc.err, got, tc.want)
		}
	}
	if CodeOf(os.ErrNotExist) != Failure {
		t.Error("a plain error has code failure")
	}
}
