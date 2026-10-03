package graph_test

import (
	"testing"

	"github.com/shikejing/material-graph-compiler-go/graph"
)

func TestGraphKeyOrderIndependent(t *testing.T) {
	v1 := graph.FloatValue(1)
	v2 := graph.FloatValue(2)
	mk := func() *graph.Graph {
		return &graph.Graph{
			Nodes: []graph.Node{
				{ID: "a", Type: "const", Overrides: map[graph.PortID]*graph.Value{"value": &v1}},
				{ID: "b", Type: "const", Overrides: map[graph.PortID]*graph.Value{"value": &v2}},
			},
			Edges: []graph.Edge{
				{From: graph.PortRef{Node: "a", Port: "out"}, To: graph.PortRef{Node: "b", Port: "in"}},
			},
			OutputNode: "b", OutputPort: "out",
		}
	}
	g1 := mk()
	g2 := mk()
	// 打乱切片顺序。
	g2.Nodes[0], g2.Nodes[1] = g2.Nodes[1], g2.Nodes[0]
	if g1.Key() != g2.Key() {
		t.Fatal("graph key must not depend on slice order")
	}
	// 内容变化必须改变键。
	g3 := mk()
	v3 := graph.FloatValue(3)
	g3.Nodes[0].Overrides["value"] = &v3
	if g1.Key() == g3.Key() {
		t.Fatal("graph key must change with content")
	}
}

func TestConvert(t *testing.T) {
	if _, ok := graph.Convert(graph.FloatValue(1), graph.TypeVec3); !ok {
		t.Fatal("float should convert to vec3")
	}
	if _, ok := graph.Convert(graph.VecValue(graph.TypeVec3, 1, 2, 3), graph.TypeFloat); ok {
		t.Fatal("vec3 must not convert to float")
	}
	c, ok := graph.Convert(graph.VecValue(graph.TypeVec3, 1, 2, 3), graph.TypeColor)
	if !ok || len(c.Vec) != 4 || c.Vec[3] != 1 {
		t.Fatalf("vec3->color should pad alpha=1, got %+v", c)
	}
}
