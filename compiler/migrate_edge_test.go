package compiler

import "testing"

// TestMigrationDefaultChange verifies that a default value change applies to
// non-overridden ports while explicit overrides are preserved.
func TestMigrationDefaultChange(t *testing.T) {
	lib := DefaultLibrary()
	lib.Register(&NodeDef{
		Type:    "add",
		Version: 2,
		Inputs:  []PortDef{{Name: "a", Type: TypeFloat, Default: FloatValue(0)}, {Name: "b", Type: TypeFloat, Default: FloatValue(0.5)}},
		Outputs: []PortDef{{Name: "out", Type: TypeFloat}},
		Emit: func(c *EmitContext) map[string]string {
			return map[string]string{"out": c.InputExpr("a") + " + " + c.InputExpr("b")}
		},
		Eval: func(inputs []Value) []Value {
			return []Value{FloatValue(inputs[0].Float + inputs[1].Float)}
		},
	})
	lib.RegisterMigration(Migration{
		NodeType:    "add",
		FromVersion: 1,
		ToVersion:   2,
		Defaults:    map[string]Value{"b": FloatValue(0.5)},
	})

	// Graph with "add" at v1: a is overridden to 1, b is not (uses default).
	g := NewGraph()
	g.AddNode(&Node{ID: "c1", Type: "constant", Inputs: map[string]Value{"value": FloatValue(1)}})
	g.AddNode(&Node{ID: "add1", Type: "add", TypeVersion: 1, Inputs: map[string]Value{"a": FloatValue(1)}})
	g.AddNode(&Node{ID: "out", Type: OutputNodeType})
	g.AddEdge(&Edge{FromNode: "c1", FromPort: "out", ToNode: "add1", ToPort: "a"})
	g.AddEdge(&Edge{FromNode: "add1", FromPort: "out", ToNode: "out", ToPort: "baseColor"})
	g.OutputNode = "out"

	s := NewSession(lib, 64, 64)
	res := s.Compile(g)
	if !res.Success {
		t.Fatalf("compile failed: %v", res.Diagnostics)
	}
	// a is overridden to 1 (but also connected to c1=1, so 1). b uses new
	// default 0.5. Result = 1 + 0.5 = 1.5.
	if !containsStr(res.Program.Code, "1.5") {
		t.Errorf("expected 1.5 (overridden a=1 + new default b=0.5):\n%s", res.Program.Code)
	}
}

// TestMigrationVersionGap verifies that a missing migration path is rejected.
func TestMigrationVersionGap(t *testing.T) {
	lib := DefaultLibrary()
	lib.Register(&NodeDef{
		Type:    "add",
		Version: 3,
		Inputs:  []PortDef{{Name: "a", Type: TypeFloat, Default: FloatValue(0)}},
		Outputs: []PortDef{{Name: "out", Type: TypeFloat}},
		Emit:    func(c *EmitContext) map[string]string { return map[string]string{"out": c.InputExpr("a")} },
	})
	// Only register v1->v2, but the node is at v1 and the def is v3.
	lib.RegisterMigration(Migration{
		NodeType:    "add",
		FromVersion: 1,
		ToVersion:   2,
		Renames:     map[string]string{"a": "x"},
	})

	g := NewGraph()
	g.AddNode(&Node{ID: "add1", Type: "add", TypeVersion: 1})
	g.AddNode(&Node{ID: "out", Type: OutputNodeType})
	g.AddEdge(&Edge{FromNode: "add1", FromPort: "out", ToNode: "out", ToPort: "baseColor"})
	g.OutputNode = "out"

	s := NewSession(lib, 64, 64)
	res := s.Compile(g)
	if res.Success {
		t.Fatal("expected migration failure for version gap")
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Kind == DiagMigrationFailed {
			found = true
		}
	}
	if !found {
		t.Errorf("no migration-failed diagnostic: %v", res.Diagnostics)
	}
}

// TestMigrationNewerVersion verifies that a node newer than the library is
// rejected.
func TestMigrationNewerVersion(t *testing.T) {
	lib := DefaultLibrary()
	lib.Register(&NodeDef{
		Type:    "add",
		Version: 2,
		Inputs:  []PortDef{{Name: "a", Type: TypeFloat, Default: FloatValue(0)}},
		Outputs: []PortDef{{Name: "out", Type: TypeFloat}},
		Emit:    func(c *EmitContext) map[string]string { return map[string]string{"out": c.InputExpr("a")} },
	})

	g := NewGraph()
	g.AddNode(&Node{ID: "add1", Type: "add", TypeVersion: 5}) // newer than library v2
	g.AddNode(&Node{ID: "out", Type: OutputNodeType})
	g.AddEdge(&Edge{FromNode: "add1", FromPort: "out", ToNode: "out", ToPort: "baseColor"})
	g.OutputNode = "out"

	s := NewSession(lib, 64, 64)
	res := s.Compile(g)
	if res.Success {
		t.Fatal("expected migration failure for newer version")
	}
}

// TestMigrationOutputRename verifies that output port renames are applied to
// edges.
func TestMigrationOutputRename(t *testing.T) {
	lib := DefaultLibrary()
	lib.Register(&NodeDef{
		Type:    "split",
		Version: 2,
		Inputs:  []PortDef{{Name: "in", Type: TypeVec3, Default: Vec3Value(0, 0, 0)}},
		Outputs: []PortDef{{Name: "r", Type: TypeFloat}, {Name: "g", Type: TypeFloat}, {Name: "b", Type: TypeFloat}},
		Emit: func(c *EmitContext) map[string]string {
			in := c.InputExpr("in")
			return map[string]string{"r": in + ".x", "g": in + ".y", "b": in + ".z"}
		},
	})
	lib.RegisterMigration(Migration{
		NodeType:      "split",
		FromVersion:   1,
		ToVersion:     2,
		OutputRenames: map[string]string{"x": "r", "y": "g", "z": "b"},
	})

	g := NewGraph()
	g.AddNode(&Node{ID: "sp", Type: "split", TypeVersion: 1})
	g.AddNode(&Node{ID: "out", Type: OutputNodeType})
	// Connect old output port "x" to the output node.
	g.AddEdge(&Edge{FromNode: "sp", FromPort: "x", ToNode: "out", ToPort: "baseColor"})
	g.OutputNode = "out"

	s := NewSession(lib, 64, 64)
	res := s.Compile(g)
	if !res.Success {
		t.Fatalf("compile failed: %v", res.Diagnostics)
	}
}
