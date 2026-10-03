package compiler_test

import (
	"strings"
	"testing"

	"github.com/shikejing/material-graph-compiler-go/compiler"
	"github.com/shikejing/material-graph-compiler-go/graph"
	"github.com/shikejing/material-graph-compiler-go/internal/testkit"
)

func TestConstantFolding(t *testing.T) {
	lib := testkit.NewLibrary()
	// 样例图中 c1,c2,sum,tex,prod 全是常量可折叠节点。
	prog, err := compiler.Compile(testkit.SampleGraph(), lib, 1, compiler.Options{Optimize: true})
	if err != nil {
		t.Fatal(err)
	}
	folded := map[graph.NodeID]bool{}
	for _, e := range prog.Eliminated {
		folded[e.Node] = true
		if e.Reason == "" {
			t.Fatalf("elimination of %s has no reason", e.Node)
		}
	}
	for _, id := range []graph.NodeID{"c1", "c2", "tex", "sum", "prod"} {
		if !folded[id] {
			t.Fatalf("expected %s to be constant-folded, eliminations: %+v", id, prog.Eliminated)
		}
	}
	// 折叠后 metallic 应为常量 (2+3)*4 = 20。
	if !strings.Contains(prog.Source, "material(vec4(1, 1, 1, 1), 20)") {
		t.Fatalf("expected folded constant in output:\n%s", prog.Source)
	}
	// 输出契约不变。
	if !strings.Contains(prog.Source, "output = v_mat_out;") {
		t.Fatalf("output contract changed:\n%s", prog.Source)
	}
}

func TestUnreachableElimination(t *testing.T) {
	lib := testkit.NewLibrary()
	g := testkit.SampleGraph()
	// 加一个与输出无关且不可折叠的节点（mix 无 Fold 规则）。
	g.Nodes = append(g.Nodes, graph.Node{ID: "dead", Type: "mix"})
	prog, err := compiler.Compile(g, lib, 1, compiler.Options{Optimize: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range prog.Eliminated {
		if e.Node == "dead" && strings.Contains(e.Reason, "unreachable") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 'dead' eliminated as unreachable: %+v", prog.Eliminated)
	}
	if strings.Contains(prog.Source, "v_dead") {
		t.Fatalf("dead node leaked into output:\n%s", prog.Source)
	}
}

func TestOptimizePreservesSemantics(t *testing.T) {
	lib := testkit.NewLibrary()
	// 不可折叠的图：material.baseColor 来自 mix（无 Fold），优化前后可观测表达式一致。
	g := testkit.SampleGraph()
	g.Nodes = append(g.Nodes, graph.Node{ID: "tint", Type: "mix"})
	g.Edges = append(g.Edges, graph.Edge{
		From: graph.PortRef{Node: "tint", Port: "out"},
		To:   graph.PortRef{Node: "mat", Port: "baseColor"},
	})
	plain, err := compiler.Compile(g, lib, 1, compiler.Options{Optimize: false})
	if err != nil {
		t.Fatal(err)
	}
	opt, err := compiler.Compile(g, lib, 1, compiler.Options{Optimize: true})
	if err != nil {
		t.Fatal(err)
	}
	// 可观测契约：输出语句一致，mix 行一致。
	if !strings.Contains(opt.Source, "output = v_mat_out;") {
		t.Fatalf("output contract changed by optimization")
	}
	extract := func(src, prefix string) string {
		for _, l := range strings.Split(src, "\n") {
			if strings.HasPrefix(l, prefix) {
				return l
			}
		}
		return ""
	}
	if extract(plain.Source, "let v_tint_out") != extract(opt.Source, "let v_tint_out") {
		t.Fatalf("observable expression changed:\nplain: %s\nopt:   %s",
			extract(plain.Source, "let v_tint_out"), extract(opt.Source, "let v_tint_out"))
	}
	// 优化版应折叠 c1/c2/sum/tex/prod（mat.metallic 变常量）。
	if !strings.Contains(opt.Source, "material(v_tint_out, 20)") {
		t.Fatalf("expected folded metallic:\n%s", opt.Source)
	}
}
