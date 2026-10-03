package compiler

import "testing"

// TestMigrationPortRename verifies that port renames are applied during
// migration and the graph compiles correctly.
func TestMigrationPortRename(t *testing.T) {
	lib := DefaultLibrary()
	lib.Register(&NodeDef{
		Type:    "add",
		Version: 2,
		Inputs:  []PortDef{{Name: "x", Type: TypeFloat, Default: FloatValue(0)}, {Name: "y", Type: TypeFloat, Default: FloatValue(0)}},
		Outputs: []PortDef{{Name: "out", Type: TypeFloat}},
		Emit: func(c *EmitContext) map[string]string {
			return map[string]string{"out": c.InputExpr("x") + " + " + c.InputExpr("y")}
		},
		Eval: func(inputs []Value) []Value {
			return []Value{FloatValue(inputs[0].Float + inputs[1].Float)}
		},
	})
	lib.RegisterMigration(Migration{
		NodeType:    "add",
		FromVersion: 1,
		ToVersion:   2,
		Renames:     map[string]string{"a": "x", "b": "y"},
	})

	// Build a graph with an "add" node at v1 using old port names a, b.
	g := NewGraph()
	g.AddNode(&Node{ID: "c1", Type: "constant", Inputs: map[string]Value{"value": FloatValue(0.5)}})
	g.AddNode(&Node{ID: "c2", Type: "constant", Inputs: map[string]Value{"value": FloatValue(0.25)}})
	g.AddNode(&Node{ID: "add1", Type: "add", TypeVersion: 1})
	g.AddNode(&Node{ID: "out", Type: OutputNodeType})
	g.AddEdge(&Edge{FromNode: "c1", FromPort: "out", ToNode: "add1", ToPort: "a"})
	g.AddEdge(&Edge{FromNode: "c2", FromPort: "out", ToNode: "add1", ToPort: "b"})
	g.AddEdge(&Edge{FromNode: "add1", FromPort: "out", ToNode: "out", ToPort: "baseColor"})
	g.OutputNode = "out"

	s := NewSession(lib, 64, 64)
	res := s.Compile(g)
	if !res.Success {
		t.Fatalf("compile failed: %v", res.Diagnostics)
	}
	// The add node should be folded (0.5 + 0.25 = 0.75).
	if !containsStr(res.Program.Code, "0.75") {
		t.Errorf("expected folded result 0.75:\n%s", res.Program.Code)
	}
}

// TestMigrationPortAdded verifies that added ports use their defaults.
func TestMigrationPortAdded(t *testing.T) {
	lib := DefaultLibrary()
	lib.Register(&NodeDef{
		Type:    "add",
		Version: 2,
		Inputs:  []PortDef{{Name: "a", Type: TypeFloat, Default: FloatValue(0)}, {Name: "b", Type: TypeFloat, Default: FloatValue(0)}, {Name: "bias", Type: TypeFloat, Default: FloatValue(0.1)}},
		Outputs: []PortDef{{Name: "out", Type: TypeFloat}},
		Emit: func(c *EmitContext) map[string]string {
			return map[string]string{"out": c.InputExpr("a") + " + " + c.InputExpr("b") + " + " + c.InputExpr("bias")}
		},
		Eval: func(inputs []Value) []Value {
			return []Value{FloatValue(inputs[0].Float + inputs[1].Float + inputs[2].Float)}
		},
	})
	lib.RegisterMigration(Migration{
		NodeType:    "add",
		FromVersion: 1,
		ToVersion:   2,
		Added:       map[string]Value{"bias": FloatValue(0.1)},
	})

	g := NewGraph()
	g.AddNode(&Node{ID: "c1", Type: "constant", Inputs: map[string]Value{"value": FloatValue(0.5)}})
	g.AddNode(&Node{ID: "add1", Type: "add", TypeVersion: 1})
	g.AddNode(&Node{ID: "out", Type: OutputNodeType})
	g.AddEdge(&Edge{FromNode: "c1", FromPort: "out", ToNode: "add1", ToPort: "a"})
	g.AddEdge(&Edge{FromNode: "add1", FromPort: "out", ToNode: "out", ToPort: "baseColor"})
	g.OutputNode = "out"

	s := NewSession(lib, 64, 64)
	res := s.Compile(g)
	if !res.Success {
		t.Fatalf("compile failed: %v", res.Diagnostics)
	}
	// 0.5 + 0 (default b) + 0.1 (default bias) = 0.6.
	if !containsStr(res.Program.Code, "0.6") {
		t.Errorf("expected folded result 0.6:\n%s", res.Program.Code)
	}
}

// TestMigrationRemovedPortRejected verifies that a removed port with an explicit
// value is rejected.
func TestMigrationRemovedPortRejected(t *testing.T) {
	lib := DefaultLibrary()
	lib.Register(&NodeDef{
		Type:    "old",
		Version: 2,
		Inputs:  []PortDef{{Name: "x", Type: TypeFloat, Default: FloatValue(0)}},
		Outputs: []PortDef{{Name: "out", Type: TypeFloat}},
		Emit:    func(c *EmitContext) map[string]string { return map[string]string{"out": c.InputExpr("x")} },
	})
	lib.RegisterMigration(Migration{
		NodeType:    "old",
		FromVersion: 1,
		ToVersion:   2,
		Removed:     []string{"legacy"},
	})

	g := NewGraph()
	g.AddNode(&Node{ID: "old1", Type: "old", TypeVersion: 1, Inputs: map[string]Value{"legacy": FloatValue(1)}})
	g.AddNode(&Node{ID: "out", Type: OutputNodeType})
	g.AddEdge(&Edge{FromNode: "old1", FromPort: "out", ToNode: "out", ToPort: "baseColor"})
	g.OutputNode = "out"

	s := NewSession(lib, 64, 64)
	res := s.Compile(g)
	if res.Success {
		t.Fatal("expected migration failure for removed port with explicit value")
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Kind == DiagMigrationFailed && d.NodeID == "old1" && d.Port == "legacy" {
			found = true
		}
	}
	if !found {
		t.Errorf("no migration-failed diagnostic for removed port: %v", res.Diagnostics)
	}
}

// TestMigrationRemovedPortConnectedRejected verifies that a removed port with a
// connection is rejected.
func TestMigrationRemovedPortConnectedRejected(t *testing.T) {
	lib := DefaultLibrary()
	lib.Register(&NodeDef{
		Type:    "old",
		Version: 2,
		Inputs:  []PortDef{{Name: "x", Type: TypeFloat, Default: FloatValue(0)}},
		Outputs: []PortDef{{Name: "out", Type: TypeFloat}},
		Emit:    func(c *EmitContext) map[string]string { return map[string]string{"out": c.InputExpr("x")} },
	})
	lib.RegisterMigration(Migration{
		NodeType:    "old",
		FromVersion: 1,
		ToVersion:   2,
		Removed:     []string{"legacy"},
	})

	g := NewGraph()
	g.AddNode(&Node{ID: "c1", Type: "constant", Inputs: map[string]Value{"value": FloatValue(0.5)}})
	g.AddNode(&Node{ID: "old1", Type: "old", TypeVersion: 1})
	g.AddNode(&Node{ID: "out", Type: OutputNodeType})
	g.AddEdge(&Edge{FromNode: "c1", FromPort: "out", ToNode: "old1", ToPort: "legacy"})
	g.AddEdge(&Edge{FromNode: "old1", FromPort: "out", ToNode: "out", ToPort: "baseColor"})
	g.OutputNode = "out"

	s := NewSession(lib, 64, 64)
	res := s.Compile(g)
	if res.Success {
		t.Fatal("expected migration failure for removed port with connection")
	}
}

// TestLibraryVersionBumpPreservesCache verifies that bumping the library version
// changes the cache key (so old results remain valid and queryable).
func TestLibraryVersionBumpPreservesCache(t *testing.T) {
	lib := DefaultLibrary()
	s := NewSession(lib, 64, 64)
	g := buildIncrementalGraph()
	r1 := s.Compile(g)
	if !r1.Success {
		t.Fatalf("first compile failed: %v", r1.Diagnostics)
	}
	key1 := r1.GraphKey

	// Bump the library version.
	lib.SetVersion(2)
	r2 := s.Compile(g)
	if !r2.Success {
		t.Fatalf("second compile failed: %v", r2.Diagnostics)
	}
	key2 := r2.GraphKey

	if key1 == key2 {
		t.Errorf("graph key should change with library version: %s vs %s", key1, key2)
	}
	// The old result should still be queryable in the cache.
	if cached, ok := s.results.Get(key1); !ok || cached.Program.Code != r1.Program.Code {
		t.Errorf("old result should remain cached and queryable")
	}
}
