package policy

import (
	"encoding/json"
	"fmt"
	"github.com/bizshuk/agentsdk/tools/dependency-analyzer/internal/graph"
	"os"
	"sort"
	"strings"
)

type Config struct {
	Layers     map[string]Layer  `json:"layers"`
	Forbidden  []Forbidden       `json:"forbidden"`
	Heavy      map[string]int    `json:"heavy"`
	Thresholds map[string]int    `json:"thresholds"`
	Severity   map[string]string `json:"severity"`
}
type Layer struct {
	Packages []string `json:"packages"`
	Allow    []string `json:"allow"`
}
type Forbidden struct {
	From string `json:"from"`
	To   string `json:"to"`
}
type Diagnostic struct {
	Code       string   `json:"code,omitempty"`
	Severity   string   `json:"severity,omitempty"`
	Message    string   `json:"message,omitempty"`
	Provenance string   `json:"provenance,omitempty"`
	Evidence   []string `json:"evidence,omitempty"`
}

func Load(path string) (Config, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return Config{}, e
	}
	var c Config
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if e = d.Decode(&c); e != nil {
		return Config{}, fmt.Errorf("decode policy: %w", e)
	}
	return c, nil
}
func Evaluate(a graph.Analysis, c Config) []Diagnostic {
	var out []Diagnostic
	for _, cy := range graph.Cycles(a.ModuleEdges) {
		out = append(out, Diagnostic{"module-cycle", severity("module-cycle", c), "module cycle: " + strings.Join(cy, " -> "), "go-tool-fact", cy})
	}
	for _, e := range a.ModuleEdges {
		for _, f := range c.Forbidden {
			if match(e.From, f.From) && match(e.To, f.To) {
				out = append(out, Diagnostic{"layer-forbidden", severity("layer-forbidden", c), e.From + " imports forbidden " + e.To, "policy-heuristic", e.Evidence})
			}
		}
	}
	for from, l := range c.Layers {
		for _, to := range graph.Reachable(a.ModuleEdges, from) {
			allowed := false
			for _, p := range l.Allow {
				if match(to, p) {
					allowed = true
				}
			}
			if !allowed {
				out = append(out, Diagnostic{"layer-forbidden", severity("layer-forbidden", c), from + " reaches disallowed layer " + to, "policy-heuristic", graph.ShortestPath(a.ModuleEdges, from, to)})
			}
		}
	}
	for heavy, weight := range c.Heavy {
		if weight < c.Thresholds["heavy_path"] {
			continue
		}
		for _, m := range a.Modules {
			path := graph.ShortestPath(a.ModuleEdges, m.Path, heavy)
			if len(path) > 1 {
				out = append(out, Diagnostic{"heavy-path", severity("heavy-path", c), m.Path + " reaches heavy module " + heavy, "policy-heuristic", path})
			}
		}
	}
	selected := map[string]string{}
	for _, m := range a.Modules {
		if m.Version != "" {
			selected[m.Path] = m.Version
		}
	}
	for _, d := range a.Direct {
		if d.Direct && d.Owner != "" && selected[d.Path] != "" && selected[d.Path] != d.Version {
			out = append(out, Diagnostic{"version-divergence", severity("version-divergence", c), d.Owner + " selects " + d.Path + " at " + selected[d.Path] + " but requires " + d.Version, "go-tool-fact", nil})
		}
	}
	for _, d := range a.Direct {
		if !d.Direct {
			continue
		}
		used := false
		for _, e := range a.ModuleEdges {
			if e.From == d.Owner && e.To == d.Path {
				used = true
			}
		}
		if !used {
			out = append(out, Diagnostic{Code: "unused-direct-candidate", Severity: severity("unused-direct-candidate", c), Message: d.Owner + " directly requires " + d.Path + " but package imports did not observe it", Provenance: "go-tool-fact", Evidence: []string{"go list -deps is package/import based; candidate is not safe to remove automatically"}})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Code != out[j].Code {
			return out[i].Code < out[j].Code
		}
		return out[i].Message < out[j].Message
	})
	return out
}
func severity(code string, c Config) string {
	if s := c.Severity[code]; s != "" {
		return s
	}
	return "warning"
}
func match(v, p string) bool { return v == p || strings.HasPrefix(v, strings.TrimSuffix(p, "*")) }
