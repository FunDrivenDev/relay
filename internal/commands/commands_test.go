package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FunDrivenDev/relay/internal/cli"
	"github.com/FunDrivenDev/relay/internal/fixture"
)

// TestRegistrySpecs is the "every command has help and a JSON input spec"
// lint (just lint runs it): it walks every entity, the test-only fixture
// included, and fails on any misdeclared command.
func TestRegistrySpecs(t *testing.T) {
	t.Parallel()
	entities := append(Entities(), fixture.Entity)
	app := cli.NewApp("relay", "test", entities)
	seen := map[string]bool{}
	for _, e := range entities {
		if seen[e.Name] {
			t.Errorf("entity %s registered twice", e.Name)
		}
		seen[e.Name] = true
		for _, p := range e.Problems() {
			t.Error(p)
		}
	}
	for _, c := range app.Commands() {
		spec := c.InputSpec()
		if spec.Summary == "" || strings.TrimSpace(c.Help) == "" {
			t.Errorf("%s: no summary or help", c.Name())
		}
		if !c.Raw && spec.Output == "" {
			t.Errorf("%s: no output description", c.Name())
		}
		for _, p := range c.Params() {
			if strings.TrimSpace(p.Help) == "" {
				t.Errorf("%s: param %s has no help", c.Name(), p.Name)
			}
		}
		if _, err := json.Marshal(spec); err != nil {
			t.Errorf("%s: input spec: %v", c.Name(), err)
		}
	}
}

// TestRegistryDocs checks that every entity has its page, listed in the
// docs index: docs/<entity>.md and a line in docs/README.md.
func TestRegistryDocs(t *testing.T) {
	t.Parallel()
	docs := filepath.Join("..", "..", "docs")
	index, err := os.ReadFile(filepath.Join(docs, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range Entities() {
		page := e.Name + ".md"
		if _, err := os.Stat(filepath.Join(docs, page)); err != nil {
			t.Errorf("entity %s has no page docs/%s", e.Name, page)
		}
		if !strings.Contains(string(index), "("+page+")") {
			t.Errorf("docs/README.md does not link %s", page)
		}
	}
}
