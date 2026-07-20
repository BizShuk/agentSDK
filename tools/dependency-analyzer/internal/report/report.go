package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/bizshuk/agentsdk/tools/dependency-analyzer/internal/graph"
	"github.com/bizshuk/agentsdk/tools/dependency-analyzer/internal/policy"
	"io"
	"sort"
	"strings"
)

type Document struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Completeness  string              `json:"completeness"`
	IncludeTests  bool                `json:"includeTests"`
	Analysis      graph.Analysis      `json:"analysis"`
	Diagnostics   []policy.Diagnostic `json:"diagnostics"`
}

func RenderText(w io.Writer, a graph.Analysis, d []policy.Diagnostic) error {
	if _, err := fmt.Fprintf(w, "Modules: %d\nPackages: %d\nModule edges: %d\nPackage edges: %d\n", len(a.Modules), len(a.Packages), len(a.ModuleEdges), len(a.PackageEdges)); err != nil {
		return err
	}
	for _, x := range d {
		if _, err := fmt.Fprintf(w, "[%s] %s (%s)\n", x.Severity, x.Message, x.Provenance); err != nil {
			return err
		}
	}
	return nil
}
func RenderJSON(w io.Writer, a graph.Analysis, d []policy.Diagnostic, indent string) error {
	enc := json.NewEncoder(w)
	if indent != "" {
		enc.SetIndent("", indent)
	}
	return enc.Encode(Document{SchemaVersion: 1, Completeness: "go-tool", Analysis: a, Diagnostics: d})
}
func RenderMermaid(w io.Writer, a graph.Analysis) error {
	if _, err := fmt.Fprintln(w, "flowchart LR"); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, m := range a.Modules {
		key := id(m.Path)
		if seen[key] {
			continue
		}
		seen[key] = true
		if _, err := fmt.Fprintf(w, "  %s[\"%s\"]\n", key, strings.ReplaceAll(m.Path, "\"", "'")); err != nil {
			return err
		}
	}
	for _, e := range a.ModuleEdges {
		if !seen[id(e.From)] || !seen[id(e.To)] {
			continue
		}
		if _, err := fmt.Fprintf(w, "  %s --> %s\n", id(e.From), id(e.To)); err != nil {
			return err
		}
	}
	return nil
}
func id(s string) string { sum := sha256.Sum256([]byte(s)); return "m_" + hex.EncodeToString(sum[:8]) }
func SortDiagnostics(d []policy.Diagnostic) {
	sort.Slice(d, func(i, j int) bool {
		if d[i].Code != d[j].Code {
			return d[i].Code < d[j].Code
		}
		return d[i].Message < d[j].Message
	})
}
