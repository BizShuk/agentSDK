package policy

import (
	"github.com/bizshuk/agentsdk/tools/dependency-analyzer/internal/discovery"
	"github.com/bizshuk/agentsdk/tools/dependency-analyzer/internal/graph"
	"os"
	"path/filepath"
	"testing"
)

func TestEvaluateDiagnostics(t *testing.T) {
	a := graph.Analysis{Modules: []graph.ModuleNode{{Path: "a", Version: "v2"}, {Path: "b", Version: "v2"}}, ModuleEdges: []graph.Edge{{From: "a", To: "b", Evidence: []string{"a/p -> b/q"}}, {From: "b", To: "a"}}, Direct: []discovery.Requirement{{Owner: "a", Path: "b", Version: "v1", Direct: true}, {Owner: "a", Path: "c", Version: "v1", Direct: true}}}
	d := Evaluate(a, Config{Forbidden: []Forbidden{{From: "a", To: "b"}}, Heavy: map[string]int{"b": 10}, Thresholds: map[string]int{"heavy_path": 5}})
	seen := map[string]bool{}
	for _, x := range d {
		seen[x.Code] = true
		if x.Provenance == "" {
			t.Fatal(x)
		}
	}
	for _, code := range []string{"module-cycle", "layer-forbidden", "heavy-path", "version-divergence", "unused-direct-candidate"} {
		if !seen[code] {
			t.Errorf("missing %s", code)
		}
	}
}
func TestLoadRejectsUnknown(t *testing.T) {
	p := filepath.Join(t.TempDir(), "p.json")
	if err := os.WriteFile(p, []byte(`{"unknown":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("expected error")
	}
}
