package graph

import (
	"github.com/bizshuk/agentsdk/tools/dependency-analyzer/internal/discovery"
	"reflect"
	"testing"
)

func TestAlgorithms(t *testing.T) {
	edges := []Edge{{"a", "b", nil}, {"b", "c", nil}, {"c", "a", nil}, {"c", "d", nil}}
	if got := Cycles(edges); !reflect.DeepEqual(got, [][]string{{"a", "b", "c"}}) {
		t.Fatalf("cycles %#v", got)
	}
	if got := Reachable(edges, "a"); !reflect.DeepEqual(got, []string{"b", "c", "d"}) {
		t.Fatalf("reachable %#v", got)
	}
	if got := ShortestPath(edges, "a", "d"); !reflect.DeepEqual(got, []string{"a", "b", "c", "d"}) {
		t.Fatalf("path %#v", got)
	}
}
func TestBuildDeduplicatesAndPreservesEvidence(t *testing.T) {
	s := discovery.Snapshot{Modules: []discovery.Module{{Path: "m1"}, {Path: "m2"}}, Packages: []discovery.Package{{ImportPath: "m1/p", Module: "m1", Imports: []string{"m2/q"}}, {ImportPath: "m1/x", Module: "m1", Imports: []string{"m2/q"}}, {ImportPath: "m2/q", Module: "m2"}}}
	a := Build(s)
	if len(a.ModuleEdges) != 1 || len(a.ModuleEdges[0].Evidence) != 2 {
		t.Fatalf("edges %#v", a.ModuleEdges)
	}
}
func TestBuildDeterministic(t *testing.T) {
	a := Build(discovery.Snapshot{Modules: []discovery.Module{{Path: "b"}, {Path: "a"}}})
	if a.Modules[0].Path != "a" {
		t.Fatal(a.Modules)
	}
}
