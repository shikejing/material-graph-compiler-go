package compiler_test

import (
	"testing"

	"github.com/shikejing/material-graph-compiler-go/cache"
	"github.com/shikejing/material-graph-compiler-go/compiler"
	"github.com/shikejing/material-graph-compiler-go/graph"
	"github.com/shikejing/material-graph-compiler-go/internal/testkit"
)

func sortedSet(ids []graph.NodeID) map[graph.NodeID]bool {
	m := make(map[graph.NodeID]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

func TestIncrementalMatchesFullRecompile(t *testing.T) {
	lib := testkit.NewLibrary()
	lines := cache.New[string](1024)
	opts := compiler.Options{Optimize: false}

	g := testkit.SampleGraph()
	first, err := compiler.CompileIncremental(g, lib, 1, opts, lines)
	if err != nil {
		t.Fatal(err)
	}
	// 首次编译：全部节点都是重编。
	if len(first.Reused) != 0 {
		t.Fatalf("first compile should reuse nothing, reused %v", first.Reused)
	}
	if len(first.Rebuilt) != len(g.Nodes) {
		t.Fatalf("first compile should rebuild all %d nodes, got %v", len(g.Nodes), first.Rebuilt)
	}

	// 局部改动：修改 c1 的常量。
	g2 := testkit.SampleGraph()
	newVal := graph.FloatValue(10)
	g2.Nodes[0].Overrides["value"] = &newVal

	second, err := compiler.CompileIncremental(g2, lib, 1, opts, lines)
	if err != nil {
		t.Fatal(err)
	}
	// 与全量重编逐字节一致。
	full, err := compiler.Compile(g2, lib, 1, opts)
	if err != nil {
		t.Fatal(err)
	}
	if second.Program.Source != full.Source {
		t.Fatalf("incremental != full:\n--- incremental ---\n%s\n--- full ---\n%s", second.Program.Source, full.Source)
	}
	// 复用/重编集合：c1 变化会传播到下游 sum、prod、mat；c2、tex 不受影响。
	reused := sortedSet(second.Reused)
	rebuilt := sortedSet(second.Rebuilt)
	for _, id := range []graph.NodeID{"c2", "tex"} {
		if !reused[id] {
			t.Fatalf("expected %s reused; reused=%v rebuilt=%v", id, second.Reused, second.Rebuilt)
		}
	}
	for _, id := range []graph.NodeID{"c1", "sum", "prod", "mat"} {
		if !rebuilt[id] {
			t.Fatalf("expected %s rebuilt; reused=%v rebuilt=%v", id, second.Reused, second.Rebuilt)
		}
	}
}

func TestIncrementalWithOptimization(t *testing.T) {
	lib := testkit.NewLibrary()
	lines := cache.New[string](1024)
	opts := compiler.Options{Optimize: true}

	g := testkit.SampleGraph()
	if _, err := compiler.CompileIncremental(g, lib, 1, opts, lines); err != nil {
		t.Fatal(err)
	}
	// 加一个不可折叠的新节点（局部改动）。
	g2 := testkit.SampleGraph()
	g2.Nodes = append(g2.Nodes, graph.Node{ID: "tint", Type: "mix"})
	g2.Edges = append(g2.Edges, graph.Edge{
		From: graph.PortRef{Node: "tint", Port: "out"},
		To:   graph.PortRef{Node: "mat", Port: "baseColor"},
	})
	inc, err := compiler.CompileIncremental(g2, lib, 1, opts, lines)
	if err != nil {
		t.Fatal(err)
	}
	full, err := compiler.Compile(g2, lib, 1, opts)
	if err != nil {
		t.Fatal(err)
	}
	if inc.Program.Source != full.Source {
		t.Fatalf("incremental != full with optimization:\n--- inc ---\n%s\n--- full ---\n%s", inc.Program.Source, full.Source)
	}
	// tint 是新增节点，必须被重编。
	if !sortedSet(inc.Rebuilt)["tint"] {
		t.Fatalf("expected tint rebuilt, got %v", inc.Rebuilt)
	}
}

func TestIncrementalDeterministicAcrossRuns(t *testing.T) {
	lib := testkit.NewLibrary()
	g := testkit.SampleGraph()
	// 两个独立缓存、不同修改历史，最终同一张图必须产出相同源码。
	lines1 := cache.New[string](1024)
	lines2 := cache.New[string](1024)
	opts := compiler.Options{Optimize: true}
	if _, err := compiler.CompileIncremental(g, lib, 1, opts, lines1); err != nil {
		t.Fatal(err)
	}
	g2 := testkit.SampleGraph()
	v := graph.FloatValue(7)
	g2.Nodes[1].Overrides["value"] = &v
	a, err := compiler.CompileIncremental(g2, lib, 1, opts, lines1)
	if err != nil {
		t.Fatal(err)
	}
	b, err := compiler.CompileIncremental(g2, lib, 1, opts, lines2)
	if err != nil {
		t.Fatal(err)
	}
	if a.Program.Source != b.Program.Source {
		t.Fatalf("same graph via different incremental histories diverged:\n%s\nvs\n%s", a.Program.Source, b.Program.Source)
	}
}
