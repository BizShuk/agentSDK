package graph

import (
	"github.com/bizshuk/agentsdk/tools/dependency-analyzer/internal/discovery"
	"sort"
	"strings"
)

type ModuleNode struct {
	Path        string                 `json:"path"`
	Version     string                 `json:"version,omitempty"`
	Workspace   bool                   `json:"workspace"`
	Dir         string                 `json:"dir,omitempty"`
	GoVersion   string                 `json:"go_version,omitempty"`
	Replacement *discovery.Replacement `json:"replacement,omitempty"`
}
type PackageNode struct {
	Path     string `json:"path"`
	Module   string `json:"module"`
	Standard bool   `json:"standard"`
}
type Edge struct {
	From     string   `json:"from"`
	To       string   `json:"to"`
	Evidence []string `json:"evidence,omitempty"`
}
type Analysis struct {
	Modules      []ModuleNode            `json:"modules"`
	Packages     []PackageNode           `json:"packages"`
	ModuleEdges  []Edge                  `json:"module_edges"`
	PackageEdges []Edge                  `json:"package_edges"`
	Direct       []discovery.Requirement `json:"direct_requirements"`
}

func Build(s discovery.Snapshot) Analysis {
	a := Analysis{}
	own := map[string]string{}
	for _, m := range s.Modules {
		a.Modules = appendUniqueModule(a.Modules, ModuleNode{Path: m.Path, Version: m.Version, Workspace: m.Main, Dir: m.Dir, GoVersion: m.GoVersion, Replacement: m.Replacement})
		for _, r := range m.Requires {
			a.Direct = append(a.Direct, r)
		}
	}
	for _, p := range s.Packages {
		owner := p.Module
		if owner == "" && p.Standard {
			owner = "stdlib"
		}
		own[p.ImportPath] = owner
		a.Packages = append(a.Packages, PackageNode{p.ImportPath, owner, p.Standard})
	}
	for _, p := range s.Packages {
		from := p.Module
		if from == "" && p.Standard {
			from = "stdlib"
		}
		for _, q := range p.Imports {
			to, ok := own[q]
			if !ok {
				continue
			}
			if to != from {
				a.ModuleEdges = add(a.ModuleEdges, Edge{from, to, []string{p.ImportPath + " -> " + q}})
			} else if p.ImportPath != q {
				a.PackageEdges = add(a.PackageEdges, Edge{p.ImportPath, q, nil})
			}
		}
	}
	sort.Slice(a.Modules, func(i, j int) bool {
		if a.Modules[i].Path != a.Modules[j].Path {
			return a.Modules[i].Path < a.Modules[j].Path
		}
		return a.Modules[i].Version < a.Modules[j].Version
	})
	sort.Slice(a.Packages, func(i, j int) bool { return a.Packages[i].Path < a.Packages[j].Path })
	sort.Slice(a.ModuleEdges, func(i, j int) bool { return edgeLess(a.ModuleEdges[i], a.ModuleEdges[j]) })
	sort.Slice(a.PackageEdges, func(i, j int) bool { return edgeLess(a.PackageEdges[i], a.PackageEdges[j]) })
	sort.Slice(a.Direct, func(i, j int) bool {
		if a.Direct[i].Owner != a.Direct[j].Owner {
			return a.Direct[i].Owner < a.Direct[j].Owner
		}
		return a.Direct[i].Path < a.Direct[j].Path
	})
	return a
}
func appendUniqueModule(ms []ModuleNode, m ModuleNode) []ModuleNode {
	for _, x := range ms {
		if x == m {
			return ms
		}
	}
	return append(ms, m)
}
func edgeLess(a, b Edge) bool {
	if a.From != b.From {
		return a.From < b.From
	}
	return a.To < b.To
}

func add(es []Edge, e Edge) []Edge {
	for i := range es {
		if es[i].From == e.From && es[i].To == e.To {
			es[i].Evidence = append(es[i].Evidence, e.Evidence...)
			sort.Strings(es[i].Evidence)
			return es
		}
	}
	return append(es, e)
}
func Cycles(es []Edge) [][]string {
	adj := map[string][]string{}
	for _, e := range es {
		adj[e.From] = append(adj[e.From], e.To)
	}
	for k := range adj {
		sort.Strings(adj[k])
	}
	idx := 0
	ind, low := map[string]int{}, map[string]int{}
	stack := []string{}
	on := map[string]bool{}
	var out [][]string
	var visit func(string)
	visit = func(v string) {
		idx++
		ind[v] = idx
		low[v] = idx
		stack = append(stack, v)
		on[v] = true
		for _, w := range adj[v] {
			if ind[w] == 0 {
				visit(w)
				if low[w] < low[v] {
					low[v] = low[w]
				}
			} else if on[w] && ind[w] < low[v] {
				low[v] = ind[w]
			}
		}
		if low[v] == ind[v] {
			var c []string
			for {
				n := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				on[n] = false
				c = append(c, n)
				if n == v {
					break
				}
			}
			if len(c) > 1 {
				sort.Strings(c)
				out = append(out, c)
			}
		}
	}
	var nodes []string
	for v := range adj {
		nodes = append(nodes, v)
	}
	sort.Strings(nodes)
	for _, v := range nodes {
		if ind[v] == 0 {
			visit(v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.Join(out[i], "/") < strings.Join(out[j], "/") })
	return out
}
func Reachable(es []Edge, start string) []string {
	adj := map[string][]string{}
	for _, e := range es {
		adj[e.From] = append(adj[e.From], e.To)
	}
	seen := map[string]bool{start: true}
	q := []string{start}
	for len(q) > 0 {
		v := q[0]
		q = q[1:]
		for _, n := range adj[v] {
			if !seen[n] {
				seen[n] = true
				q = append(q, n)
			}
		}
	}
	delete(seen, start)
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
func ShortestPath(es []Edge, from, to string) []string {
	adj := map[string][]string{}
	for _, e := range es {
		adj[e.From] = append(adj[e.From], e.To)
	}
	prev := map[string]string{from: ""}
	q := []string{from}
	for len(q) > 0 {
		v := q[0]
		q = q[1:]
		if v == to {
			break
		}
		for _, n := range adj[v] {
			if _, ok := prev[n]; !ok {
				prev[n] = v
				q = append(q, n)
			}
		}
	}
	if _, ok := prev[to]; !ok {
		return nil
	}
	var p []string
	for v := to; v != ""; v = prev[v] {
		p = append(p, v)
	}
	for i, j := 0, len(p)-1; i < j; i, j = i+1, j-1 {
		p[i], p[j] = p[j], p[i]
	}
	return p
}
