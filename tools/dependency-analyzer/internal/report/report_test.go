package report

import (
	"bytes"
	"github.com/bizshuk/agentsdk/tools/dependency-analyzer/internal/graph"
	"github.com/bizshuk/agentsdk/tools/dependency-analyzer/internal/policy"
	"strings"
	"testing"
)

func TestRenderDeterministic(t *testing.T) {
	a := graph.Analysis{Modules: []graph.ModuleNode{{Path: "example.com/a"}, {Path: "example.com/b"}}, ModuleEdges: []graph.Edge{{From: "example.com/a", To: "example.com/b"}}}
	var x, y bytes.Buffer
	if err := RenderJSON(&x, a, nil, "  "); err != nil {
		t.Fatal(err)
	}
	if err := RenderJSON(&y, a, nil, "  "); err != nil {
		t.Fatal(err)
	}
	if x.String() != y.String() {
		t.Fatal("JSON changed")
	}
	var m bytes.Buffer
	if err := RenderMermaid(&m, a); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.String(), `m_036cbe16b0015b0a --> m_21e68f50264534ee`) {
		t.Fatal(m.String())
	}
}
func TestTextIncludesDiagnostics(t *testing.T) {
	var b bytes.Buffer
	RenderText(&b, graph.Analysis{}, []policy.Diagnostic{{Code: "x", Severity: "warning", Message: "msg", Provenance: "policy-heuristic"}})
	if !strings.Contains(b.String(), "policy-heuristic") {
		t.Fatal(b.String())
	}
}
