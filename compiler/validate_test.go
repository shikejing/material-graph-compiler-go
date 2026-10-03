package compiler_test

import (
	"strings"
	"testing"

	"github.com/shikejing/material-graph-compiler-go/compiler"
	"github.com/shikejing/material-graph-compiler-go/graph"
	"github.com/shikejing/material-graph-compiler-go/internal/testkit"
)

func compileErr(t *testing.T, g *graph.Graph) *graph.Error {
	t.Helper()
	lib := testkit.NewLibrary()
	prog, err := compiler.Compile(g, lib, 1, compiler.Options{Optimize: true})
	if err == nil {
		t.Fatalf("expected error, got program:\n%s", prog.Source)
	}
	if prog != nil {
		t.Fatalf("invalid graph must not produce partial output, got %+v", prog)
	}
	verr, ok := err.(*graph.Error)
	if !ok {
		t.Fatalf("expected *graph.Error, got %T: %v", err, err)
	}
	return verr
}

func hasDiag(verr *graph.Error, code string, node graph.NodeID) bool {
	for _, d := range verr.Diags {
		if d.Code == code && d.Node == node {
			return true
		}
	}
	return false
}

func TestValidGraphCompiles(t *testing.T) {
	lib := testkit.NewLibrary()
	prog, err := compiler.Compile(testkit.SampleGraph(), lib, 1, compiler.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(prog.Source, "output = v_mat_out;") {
		t.Fatalf("missing output line:\n%s", prog.Source)
	}
}

func TestCycleRejected(t *testing.T) {
	g := testkit.SampleGraph()
	// 制造环：prod.out -> sum.a（sum 已在 prod 上游）。
	g.Edges = append(g.Edges, graph.Edge{
		From: graph.PortRef{Node: "prod", Port: "out"},
		To:   graph.PortRef{Node: "sum", Port: "a"},
	})
	// 去掉原来的 c1->sum.a，避免重复入线干扰。
	g.Edges = g.Edges[1:]
	verr := compileErr(t, g)
	if !hasDiag(verr, "CYCLE", "sum") || !hasDiag(verr, "CYCLE", "prod") {
		t.Fatalf("expected CYCLE diags for sum and prod, got %+v", verr.Diags)
	}
}

func TestTypeMismatchRejected(t *testing.T) {
	g := testkit.SampleGraph()
	// material.baseColor 是 color，prod.out 是 float：float->color 兼容，
	// 反向不兼容：把 mix.out(color) 连到 mul.a(float)。
	g.Nodes = append(g.Nodes, graph.Node{ID: "m", Type: "mix"})
	g.Edges = append(g.Edges, graph.Edge{
		From: graph.PortRef{Node: "m", Port: "out"},
		To:   graph.PortRef{Node: "prod", Port: "a"},
	})
	verr := compileErr(t, g)
	found := false
	for _, d := range verr.Diags {
		if d.Code == "TYPE_MISMATCH" && d.Node == "prod" && d.Port == "a" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected TYPE_MISMATCH at prod.a, got %+v", verr.Diags)
	}
}

func TestDanglingPortRejected(t *testing.T) {
	g := testkit.SampleGraph()
	g.Edges = append(g.Edges, graph.Edge{
		From: graph.PortRef{Node: "c1", Port: "nope"},
		To:   graph.PortRef{Node: "sum", Port: "a"},
	})
	verr := compileErr(t, g)
	found := false
	for _, d := range verr.Diags {
		if d.Code == "PORT_NOT_FOUND" && d.Node == "c1" && d.Port == "nope" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected PORT_NOT_FOUND at c1.nope, got %+v", verr.Diags)
	}
}

func TestUnknownNodeTypeRejected(t *testing.T) {
	g := testkit.SampleGraph()
	g.Nodes[0].Type = "does-not-exist"
	verr := compileErr(t, g)
	if !hasDiag(verr, "UNKNOWN_NODE_TYPE", "c1") {
		t.Fatalf("expected UNKNOWN_NODE_TYPE for c1, got %+v", verr.Diags)
	}
}

func TestDiagnosticsAreSorted(t *testing.T) {
	g := testkit.SampleGraph()
	g.Nodes[0].Type = "nope1"
	g.Nodes[1].Type = "nope2"
	verr := compileErr(t, g)
	for i := 1; i < len(verr.Diags); i++ {
		if verr.Diags[i-1].Node > verr.Diags[i].Node {
			t.Fatalf("diagnostics not sorted: %+v", verr.Diags)
		}
	}
}
