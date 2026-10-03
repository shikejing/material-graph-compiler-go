package compiler

import "testing"

// newTestSession creates a session with the default library and given capacities.
func newTestSession() *Session {
	return NewSession(DefaultLibrary(), 64, 64)
}

// TestCycleDetection verifies that graphs with cycles are rejected and the
// diagnostic locates the involved nodes.
func TestCycleDetection(t *testing.T) {
	lib := DefaultLibrary()
	g := NewGraph()
	g.AddNode(&Node{ID: "a", Type: "add"})
	g.AddNode(&Node{ID: "b", Type: "multiply"})
	g.AddNode(&Node{ID: "out", Type: OutputNodeType})
	// a.out -> b.a, b.out -> a.a (cycle)
	g.AddEdge(&Edge{FromNode: "a", FromPort: "out", ToNode: "b", ToPort: "a"})
	g.AddEdge(&Edge{FromNode: "b", FromPort: "out", ToNode: "a", ToPort: "a"})
	g.AddEdge(&Edge{FromNode: "a", FromPort: "out", ToNode: "out", ToPort: "baseColor"})
	g.OutputNode = "out"

	diags := ValidateGraph(lib, g)
	if !HasError(diags) {
		t.Fatal("expected cycle error, got none")
	}
	found := false
	for _, d := range diags {
		if d.Kind == DiagCycle {
			found = true
			if d.NodeID == "" {
				t.Error("cycle diagnostic missing node ID")
			}
		}
	}
	if !found {
		t.Errorf("no cycle diagnostic found: %v", diags)
	}
}

// TestSelfCycle verifies that a self-loop is rejected.
func TestSelfCycle(t *testing.T) {
	lib := DefaultLibrary()
	g := NewGraph()
	g.AddNode(&Node{ID: "a", Type: "add"})
	g.AddNode(&Node{ID: "out", Type: OutputNodeType})
	g.AddEdge(&Edge{FromNode: "a", FromPort: "out", ToNode: "a", ToPort: "a"})
	g.AddEdge(&Edge{FromNode: "a", FromPort: "out", ToNode: "out", ToPort: "baseColor"})
	g.OutputNode = "out"

	diags := ValidateGraph(lib, g)
	if !HasError(diags) {
		t.Fatal("expected self-cycle error")
	}
}

// TestTypeMismatch verifies that incompatible port types are rejected.
func TestTypeMismatch(t *testing.T) {
	lib := DefaultLibrary()
	g := NewGraph()
	g.AddNode(&Node{ID: "tex", Type: "constant"}) // constant outputs float
	g.AddNode(&Node{ID: "out", Type: OutputNodeType})
	// float -> vec3 is compatible (splat), so this should pass.
	g.AddEdge(&Edge{FromNode: "tex", FromPort: "out", ToNode: "out", ToPort: "baseColor"})
	g.OutputNode = "out"
	if diags := ValidateGraph(lib, g); HasError(diags) {
		t.Fatalf("float->vec3 should be compatible: %v", diags)
	}

	// Now test an incompatible connection: texture -> float.
	lib2 := DefaultLibrary()
	lib2.Register(&NodeDef{
		Type:    "texnode",
		Version: 1,
		Inputs:  []PortDef{{Name: "uv", Type: TypeVec2, Default: Vec2Value(0, 0)}},
		Outputs: []PortDef{{Name: "color", Type: TypeTexture}},
		Emit:    func(c *EmitContext) map[string]string { return map[string]string{"color": "texture(u_tex)"} },
	})
	g3 := NewGraph()
	g3.AddNode(&Node{ID: "tn", Type: "texnode"})
	g3.AddNode(&Node{ID: "out", Type: OutputNodeType})
	g3.AddEdge(&Edge{FromNode: "tn", FromPort: "color", ToNode: "out", ToPort: "opacity"})
	g3.OutputNode = "out"
	diags := ValidateGraph(lib2, g3)
	if !HasError(diags) {
		t.Fatal("expected type mismatch for texture->float")
	}
	found := false
	for _, d := range diags {
		if d.Kind == DiagTypeMismatch {
			found = true
		}
	}
	if !found {
		t.Errorf("no type mismatch diagnostic: %v", diags)
	}
}

// TestPortNotFound verifies that edges to non-existent ports are rejected.
func TestPortNotFound(t *testing.T) {
	lib := DefaultLibrary()
	g := NewGraph()
	g.AddNode(&Node{ID: "a", Type: "add"})
	g.AddNode(&Node{ID: "out", Type: OutputNodeType})
	g.AddEdge(&Edge{FromNode: "a", FromPort: "nonexistent", ToNode: "out", ToPort: "baseColor"})
	g.OutputNode = "out"
	diags := ValidateGraph(lib, g)
	if !HasError(diags) {
		t.Fatal("expected port-not-found error")
	}
	found := false
	for _, d := range diags {
		if d.Kind == DiagPortNotFound {
			found = true
			if d.NodeID == "" || d.Port == "" {
				t.Error("port-not-found diagnostic missing node/port")
			}
		}
	}
	if !found {
		t.Errorf("no port-not-found diagnostic: %v", diags)
	}
}

// TestUnknownNodeType verifies that unknown node types are rejected.
func TestUnknownNodeType(t *testing.T) {
	lib := DefaultLibrary()
	g := NewGraph()
	g.AddNode(&Node{ID: "x", Type: "nonexistent"})
	g.AddNode(&Node{ID: "out", Type: OutputNodeType})
	g.AddEdge(&Edge{FromNode: "x", FromPort: "out", ToNode: "out", ToPort: "baseColor"})
	g.OutputNode = "out"
	diags := ValidateGraph(lib, g)
	if !HasError(diags) {
		t.Fatal("expected unknown node type error")
	}
}

// TestNoOutput verifies that a graph without an output node is rejected.
func TestNoOutput(t *testing.T) {
	lib := DefaultLibrary()
	g := NewGraph()
	g.AddNode(&Node{ID: "a", Type: "add"})
	diags := ValidateGraph(lib, g)
	if !HasError(diags) {
		t.Fatal("expected no-output error")
	}
}

// TestMultipleOutputs verifies that multiple output nodes are rejected.
func TestMultipleOutputs(t *testing.T) {
	lib := DefaultLibrary()
	g := NewGraph()
	g.AddNode(&Node{ID: "out1", Type: OutputNodeType})
	g.AddNode(&Node{ID: "out2", Type: OutputNodeType})
	g.OutputNode = "out1"
	diags := ValidateGraph(lib, g)
	if !HasError(diags) {
		t.Fatal("expected multiple-outputs error")
	}
}

// TestDoubleConnection verifies that an input port connected twice is rejected.
func TestDoubleConnection(t *testing.T) {
	lib := DefaultLibrary()
	g := NewGraph()
	g.AddNode(&Node{ID: "a", Type: "constant"})
	g.AddNode(&Node{ID: "b", Type: "constant"})
	g.AddNode(&Node{ID: "out", Type: OutputNodeType})
	g.AddEdge(&Edge{FromNode: "a", FromPort: "out", ToNode: "out", ToPort: "baseColor"})
	g.AddEdge(&Edge{FromNode: "b", FromPort: "out", ToNode: "out", ToPort: "baseColor"})
	g.OutputNode = "out"
	diags := ValidateGraph(lib, g)
	if !HasError(diags) {
		t.Fatal("expected double-connection error")
	}
}
