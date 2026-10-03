package compiler_test

import (
	"sync"
	"testing"

	"github.com/shikejing/material-graph-compiler-go/compiler"
	"github.com/shikejing/material-graph-compiler-go/graph"
	"github.com/shikejing/material-graph-compiler-go/internal/testkit"
)

// shuffled 返回节点与连线顺序被打乱（确定性旋转）的图副本。
func shuffled(g *graph.Graph, k int) *graph.Graph {
	out := *g
	n := len(g.Nodes)
	nodes := make([]graph.Node, n)
	for i := range g.Nodes {
		nodes[i] = g.Nodes[(i+k)%n]
	}
	m := len(g.Edges)
	edges := make([]graph.Edge, m)
	for i := range g.Edges {
		edges[i] = g.Edges[(i+2*k)%m]
	}
	out.Nodes = nodes
	out.Edges = edges
	return &out
}

func TestDeterministicAcrossTraversalOrder(t *testing.T) {
	lib := testkit.NewLibrary()
	base := testkit.SampleGraph()
	want, err := compiler.Compile(base, lib, 1, compiler.Options{Optimize: true})
	if err != nil {
		t.Fatal(err)
	}
	for k := 1; k <= 7; k++ {
		got, err := compiler.Compile(shuffled(testkit.SampleGraph(), k), lib, 1, compiler.Options{Optimize: true})
		if err != nil {
			t.Fatal(err)
		}
		if got.Source != want.Source {
			t.Fatalf("shuffle %d produced different output:\n--- want ---\n%s\n--- got ---\n%s", k, want.Source, got.Source)
		}
	}
}

func TestDeterministicAcrossConcurrency(t *testing.T) {
	lib := testkit.NewLibrary()
	want, err := compiler.Compile(testkit.SampleGraph(), lib, 1, compiler.Options{Optimize: true})
	if err != nil {
		t.Fatal(err)
	}
	const workers = 32
	results := make([]string, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p, err := compiler.Compile(shuffled(testkit.SampleGraph(), i%5), lib, 1, compiler.Options{Optimize: true})
			if err != nil {
				t.Error(err)
				return
			}
			results[i] = p.Source
		}(i)
	}
	wg.Wait()
	for i, s := range results {
		if s != want.Source {
			t.Fatalf("worker %d produced different output", i)
		}
	}
}
