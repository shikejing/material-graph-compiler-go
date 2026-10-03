// Package testkit 提供测试共用的节点库与样例图。
package testkit

import (
	"fmt"

	"github.com/shikejing/material-graph-compiler-go/graph"
	"github.com/shikejing/material-graph-compiler-go/nodelib"
)

func fport(name graph.PortID, def float64) nodelib.PortDef {
	v := graph.FloatValue(def)
	return nodelib.PortDef{Name: name, Type: graph.TypeFloat, Default: &v}
}

func out(name graph.PortID, t graph.ValueType) nodelib.PortDef {
	return nodelib.PortDef{Name: name, Type: t}
}

// LibraryV1 构造节点库 v1。
func LibraryV1() nodelib.Version {
	defs := map[string]*nodelib.NodeDef{}
	defs["const"] = &nodelib.NodeDef{
		Type:    "const",
		Inputs:  []nodelib.PortDef{fport("value", 0)},
		Outputs: []nodelib.PortDef{out("out", graph.TypeFloat)},
		Emit: func(output graph.PortID, inputs map[graph.PortID]string, ov map[graph.PortID]*graph.Value) (string, error) {
			return inputs["value"], nil
		},
		Fold: func(in map[graph.PortID]graph.Value) (graph.Value, bool) {
			return in["value"], true
		},
	}
	binop := func(typ, op string) *nodelib.NodeDef {
		return &nodelib.NodeDef{
			Type:    typ,
			Inputs:  []nodelib.PortDef{fport("a", 0), fport("b", 0)},
			Outputs: []nodelib.PortDef{out("out", graph.TypeFloat)},
			Emit: func(output graph.PortID, inputs map[graph.PortID]string, ov map[graph.PortID]*graph.Value) (string, error) {
				return fmt.Sprintf("(%s %s %s)", inputs["a"], op, inputs["b"]), nil
			},
			Fold: func(in map[graph.PortID]graph.Value) (graph.Value, bool) {
				a, b := in["a"].Float, in["b"].Float
				switch op {
				case "+":
					return graph.FloatValue(a + b), true
				case "*":
					return graph.FloatValue(a * b), true
				}
				return graph.Value{}, false
			},
		}
	}
	defs["add"] = binop("add", "+")
	defs["mul"] = binop("mul", "*")
	defs["mix"] = &nodelib.NodeDef{
		Type: "mix",
		Inputs: []nodelib.PortDef{
			{Name: "x", Type: graph.TypeColor, Default: ptr(graph.VecValue(graph.TypeColor, 0, 0, 0, 1))},
			{Name: "y", Type: graph.TypeColor, Default: ptr(graph.VecValue(graph.TypeColor, 1, 1, 1, 1))},
			fport("t", 0.5),
		},
		Outputs: []nodelib.PortDef{out("out", graph.TypeColor)},
		Emit: func(output graph.PortID, inputs map[graph.PortID]string, ov map[graph.PortID]*graph.Value) (string, error) {
			return fmt.Sprintf("mix(%s, %s, %s)", inputs["x"], inputs["y"], inputs["t"]), nil
		},
	}
	defs["material"] = &nodelib.NodeDef{
		Type: "material",
		Inputs: []nodelib.PortDef{
			{Name: "baseColor", Type: graph.TypeColor, Default: ptr(graph.VecValue(graph.TypeColor, 1, 1, 1, 1))},
			fport("metallic", 0),
		},
		Outputs: []nodelib.PortDef{out("out", graph.TypeColor)},
		Emit: func(output graph.PortID, inputs map[graph.PortID]string, ov map[graph.PortID]*graph.Value) (string, error) {
			return fmt.Sprintf("material(%s, %s)", inputs["baseColor"], inputs["metallic"]), nil
		},
	}
	defs["legacy"] = &nodelib.NodeDef{
		Type:    "legacy",
		Inputs:  []nodelib.PortDef{fport("in", 0)},
		Outputs: []nodelib.PortDef{out("out", graph.TypeFloat)},
		Emit: func(output graph.PortID, inputs map[graph.PortID]string, ov map[graph.PortID]*graph.Value) (string, error) {
			return inputs["in"], nil
		},
	}
	return nodelib.Version{Number: 1, Defs: defs}
}

// LibraryV2 构造节点库 v2：
//   - add 的输入端口 a/b 改名为 lhs/rhs（有迁移规则）；
//   - mix 的 t 默认值从 0.5 改为 0.25，迁移规则用 NewDefaults 钉住旧行为；
//   - legacy 类型被删除（无迁移规则 => 不可自动兼容）。
func LibraryV2() nodelib.Version {
	v1 := LibraryV1()
	defs := map[string]*nodelib.NodeDef{}
	for k, v := range v1.Defs {
		defs[k] = v
	}
	// add v2：端口改名。
	defs["add"] = &nodelib.NodeDef{
		Type:    "add",
		Inputs:  []nodelib.PortDef{fport("lhs", 0), fport("rhs", 0)},
		Outputs: []nodelib.PortDef{out("out", graph.TypeFloat)},
		Emit: func(output graph.PortID, inputs map[graph.PortID]string, ov map[graph.PortID]*graph.Value) (string, error) {
			return fmt.Sprintf("(%s + %s)", inputs["lhs"], inputs["rhs"]), nil
		},
		Fold: func(in map[graph.PortID]graph.Value) (graph.Value, bool) {
			return graph.FloatValue(in["lhs"].Float + in["rhs"].Float), true
		},
	}
	// mix v2：t 默认值变化。
	mix := *defs["mix"]
	mix.Inputs = []nodelib.PortDef{
		mix.Inputs[0], mix.Inputs[1], fport("t", 0.25),
	}
	defs["mix"] = &mix
	// legacy 被删除。
	delete(defs, "legacy")

	migrations := []nodelib.Migration{
		{
			Type:         "add",
			FromVersion:  1,
			RenamedPorts: map[graph.PortID]graph.PortID{"a": "lhs", "b": "rhs"},
		},
		{
			Type:        "mix",
			FromVersion: 1,
			NewDefaults: map[graph.PortID]graph.Value{"t": graph.FloatValue(0.5)},
		},
	}
	return nodelib.Version{Number: 2, Defs: defs, Migrations: migrations}
}

// NewLibrary 返回注册好 v1、v2 的节点库。
func NewLibrary() *nodelib.Library {
	lib := nodelib.NewLibrary()
	if err := lib.AddVersion(LibraryV1()); err != nil {
		panic(err)
	}
	if err := lib.AddVersion(LibraryV2()); err != nil {
		panic(err)
	}
	return lib
}

// SampleGraph 返回一张合法图：
//
//	c1(const 2) ─┐
//	             add ─┐
//	c2(const 3) ─┘    ├─► mul ──► material.metallic ──► out
//	tex(const 4) ─────┘
func SampleGraph() *graph.Graph {
	v2 := graph.FloatValue(2)
	v3 := graph.FloatValue(3)
	v4 := graph.FloatValue(4)
	return &graph.Graph{
		Nodes: []graph.Node{
			{ID: "c1", Type: "const", Overrides: map[graph.PortID]*graph.Value{"value": &v2}},
			{ID: "c2", Type: "const", Overrides: map[graph.PortID]*graph.Value{"value": &v3}},
			{ID: "tex", Type: "const", Overrides: map[graph.PortID]*graph.Value{"value": &v4}},
			{ID: "sum", Type: "add"},
			{ID: "prod", Type: "mul"},
			{ID: "mat", Type: "material"},
		},
		Edges: []graph.Edge{
			{From: graph.PortRef{Node: "c1", Port: "out"}, To: graph.PortRef{Node: "sum", Port: "a"}},
			{From: graph.PortRef{Node: "c2", Port: "out"}, To: graph.PortRef{Node: "sum", Port: "b"}},
			{From: graph.PortRef{Node: "sum", Port: "out"}, To: graph.PortRef{Node: "prod", Port: "a"}},
			{From: graph.PortRef{Node: "tex", Port: "out"}, To: graph.PortRef{Node: "prod", Port: "b"}},
			{From: graph.PortRef{Node: "prod", Port: "out"}, To: graph.PortRef{Node: "mat", Port: "metallic"}},
		},
		OutputNode: "mat",
		OutputPort: "out",
	}
}

func ptr(v graph.Value) *graph.Value { return &v }
