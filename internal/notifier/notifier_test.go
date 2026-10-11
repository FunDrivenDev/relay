package notifier

import (
	"strings"
	"testing"
)

func TestAdvice(t *testing.T) {
	t.Parallel()
	if got := (Report{}).Advice(); len(got) != 1 || got[0] != "guide: "+Guide {
		t.Errorf("no hints: %q", got)
	}
	r := Report{Hints: []string{"set notify_command", "options: x; guide: " + Guide}}
	if got := r.Advice(); len(got) != 2 {
		t.Errorf("the guide given twice: %q", got)
	}
}

func TestWhere(t *testing.T) {
	t.Parallel()
	e := Environment{OS: "linux", SSH: true, Why: "over SSH, without a display"}
	if got := e.Where(); got != "linux, headless, over SSH (over SSH, without a display)" {
		t.Errorf("headless: %q", got)
	}
	e = Environment{OS: "macos", Desktop: true, Why: "macOS"}
	if got := e.Where(); !strings.HasPrefix(got, "macos, desktop") {
		t.Errorf("desktop: %q", got)
	}
}
