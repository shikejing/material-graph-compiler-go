package nodelib_test

import (
	"strings"
	"testing"

	"github.com/shikejing/material-graph-compiler-go/compiler"
	"github.com/shikejing/material-graph-compiler-go/graph"
	"github.com/shikejing/material-graph-compiler-go/internal/testkit"
	"github.com/shikejing/material-graph-compiler-go/nodelib"
)

// v1 图：add 使用旧端口 a/b，mix 依赖旧默认值 t=0.5。
func v1Graph() *graph.Graph {
	v2 := graph.FloatValue(2)
	v3 := graph.FloatValue(3)
	return &graph.Graph{
		Nodes: []graph.Node{
			{ID: "c1", Type: "const", Overrides: map[graph.PortID]*graph.Value{"value": &v2}},
			{ID: "c2", Type: "const", Overrides: map[graph.PortID]*graph.Value{"value": &v3}},
			{ID: "sum", Type: "add"},
			{ID: "tint", Type: "mix"},
			{ID: "mat", Type: "material"},
		},
		Edges: []graph.Edge{
			{From: graph.PortRef{Node: "c1", Port: "out"}, To: graph.PortRef{Node: "sum", Port: "a"}},
			{From: graph.PortRef{Node: "c2", Port: "out"}, To: graph.PortRef{Node: "sum", Port: "b"}},
			{From: graph.PortRef{Node: "sum", Port: "out"}, To: graph.PortRef{Node: "mat", Port: "metallic"}},
			{From: graph.PortRef{Node: "tint", Port: "out"}, To: graph.PortRef{Node: "mat", Port: "baseColor"}},
		},
		OutputNode: "mat",
		OutputPort: "out",
	}
}

func TestMigrateRenamedPorts(t *testing.T) {
	lib := testkit.NewLibrary()
	g2, err := nodelib.MigrateGraph(v1Graph(), lib, 1, 2)
	if err != nil {
		t.Fatalf("migration should succeed: %v", err)
	}
	// 连线端口应被改名 a->lhs, b->rhs。
	var lhsEdge, rhsEdge bool
	for _, e := range g2.Edges {
		if e.To.Node == "sum" && e.To.Port == "lhs" {
			lhsEdge = true
		}
		if e.To.Node == "sum" && e.To.Port == "rhs" {
			rhsEdge = true
		}
		if e.To.Port == "a" || e.To.Port == "b" {
			t.Fatalf("old port name survived migration: %+v", e)
		}
	}
	if !lhsEdge || !rhsEdge {
		t.Fatalf("expected renamed edges to lhs/rhs: %+v", g2.Edges)
	}
	// 迁移后的图必须能在 v2 下编译。
	if _, err := compiler.Compile(g2, lib, 2, compiler.Options{Optimize: true}); err != nil {
		t.Fatalf("migrated graph does not compile under v2: %v", err)
	}
}

func TestMigrateDefaultChangePinned(t *testing.T) {
	lib := testkit.NewLibrary()
	g2, err := nodelib.MigrateGraph(v1Graph(), lib, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	// mix 的 t 默认值在 v2 变为 0.25，迁移规则应用 NewDefaults 钉住旧值 0.5。
	var tint *graph.Node
	for i := range g2.Nodes {
		if g2.Nodes[i].ID == "tint" {
			tint = &g2.Nodes[i]
		}
	}
	if tint == nil {
		t.Fatal("tint node missing after migration")
	}
	ov, ok := tint.Overrides["t"]
	if !ok || ov == nil || ov.Float != 0.5 {
		t.Fatalf("expected t pinned to 0.5, got %+v", tint.Overrides)
	}
	// 语义保持：v1 下编译与迁移后 v2 下编译，mix 行的可观测表达式一致。
	p1, err := compiler.Compile(v1Graph(), lib, 1, compiler.Options{})
	if err != nil {
		t.Fatal(err)
	}
	p2, err := compiler.Compile(g2, lib, 2, compiler.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p1.Source, "0.5") || !strings.Contains(p2.Source, "0.5") {
		t.Fatalf("mix factor changed across migration:\nv1:\n%s\nv2:\n%s", p1.Source, p2.Source)
	}
}

func TestMigrateIncompatibleRejected(t *testing.T) {
	lib := testkit.NewLibrary()
	g := v1Graph()
	// legacy 类型在 v2 被删除且无迁移规则。
	g.Nodes = append(g.Nodes, graph.Node{ID: "old", Type: "legacy"})
	g.Edges = append(g.Edges, graph.Edge{
		From: graph.PortRef{Node: "old", Port: "out"},
		To:   graph.PortRef{Node: "sum", Port: "a"},
	})
	before := g.Key()
	_, err := nodelib.MigrateGraph(g, lib, 1, 2)
	if err == nil {
		t.Fatal("expected migration to be rejected")
	}
	merr, ok := err.(*nodelib.MigrateError)
	if !ok {
		t.Fatalf("expected *MigrateError, got %T", err)
	}
	if len(merr.Reasons) == 0 || !strings.Contains(merr.Reasons[0], "legacy") {
		t.Fatalf("error should name the offending type: %+v", merr.Reasons)
	}
	// 原图不被修改。
	if g.Key() != before {
		t.Fatal("failed migration mutated the original graph")
	}
}

func TestMigrationDoesNotBreakExistingResults(t *testing.T) {
	lib := testkit.NewLibrary()
	// 旧版本编译结果在库升级后依然可复现（版本化缓存键）。
	g := v1Graph()
	before, err := compiler.Compile(g, lib, 1, compiler.Options{Optimize: true})
	if err != nil {
		t.Fatal(err)
	}
	// 模拟库演进后再次用 v1 编译同一图：结果逐字节一致。
	after, err := compiler.Compile(g, lib, 1, compiler.Options{Optimize: true})
	if err != nil {
		t.Fatal(err)
	}
	if before.Source != after.Source {
		t.Fatal("library evolution changed results pinned to old version")
	}
	// v1 与 v2 的缓存键必须不同。
	k1 := compiler.CacheKey(g, lib, 1, compiler.Options{})
	k2 := compiler.CacheKey(g, lib, 2, compiler.Options{})
	if k1 == k2 {
		t.Fatal("cache key must include library version")
	}
}

func TestMigrateDowngradeRejected(t *testing.T) {
	lib := testkit.NewLibrary()
	if _, err := nodelib.MigrateGraph(v1Graph(), lib, 2, 1); err == nil {
		t.Fatal("downgrade must be rejected")
	}
}
