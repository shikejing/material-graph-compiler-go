package compiler

import "testing"

// TestNoPartialResults verifies that when validation fails, no program is
// produced and the diagnostics locate the specific node and port.
func TestNoPartialResults(t *testing.T) {
	s := newTestSession()

	// Graph with a cycle: should be rejected atomically.
	g := NewGraph()
	g.AddNode(&Node{ID: "a", Type: "add"})
	g.AddNode(&Node{ID: "b", Type: "multiply"})
	g.AddNode(&Node{ID: "out", Type: OutputNodeType})
	g.AddEdge(&Edge{FromNode: "a", FromPort: "out", ToNode: "b", ToPort: "a"})
	g.AddEdge(&Edge{FromNode: "b", FromPort: "out", ToNode: "a", ToPort: "a"})
	g.AddEdge(&Edge{FromNode: "a", FromPort: "out", ToNode: "out", ToPort: "baseColor"})
	g.OutputNode = "out"

	res := s.Compile(g)
	if res.Success {
		t.Fatal("expected failure for cyclic graph")
	}
	if res.Program != nil {
		t.Error("no program should be produced on failure")
	}
	if !HasError(res.Diagnostics) {
		t.Error("expected error diagnostics")
	}
	// The cycle diagnostic should locate the involved nodes.
	found := false
	for _, d := range res.Diagnostics {
		if d.Kind == DiagCycle {
			found = true
			if d.NodeID == "" {
				t.Error("cycle diagnostic missing node ID")
			}
		}
	}
	if !found {
		t.Errorf("no cycle diagnostic: %v", res.Diagnostics)
	}
}

// TestNoPartialResultsTypeMismatch verifies that a type mismatch rejects the
// whole graph without producing a program.
func TestNoPartialResultsTypeMismatch(t *testing.T) {
	s := newTestSession()
	lib := s.Library()
	lib.Register(&NodeDef{
		Type:    "texnode",
		Version: 1,
		Inputs:  []PortDef{{Name: "uv", Type: TypeVec2, Default: Vec2Value(0, 0)}},
		Outputs: []PortDef{{Name: "color", Type: TypeTexture}},
		Emit:    func(c *EmitContext) map[string]string { return map[string]string{"color": "texture(u_tex)"} },
	})

	g := NewGraph()
	g.AddNode(&Node{ID: "tn", Type: "texnode"})
	g.AddNode(&Node{ID: "out", Type: OutputNodeType})
	g.AddEdge(&Edge{FromNode: "tn", FromPort: "color", ToNode: "out", ToPort: "opacity"})
	g.OutputNode = "out"

	res := s.Compile(g)
	if res.Success {
		t.Fatal("expected failure for type mismatch")
	}
	if res.Program != nil {
		t.Error("no program should be produced on failure")
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Kind == DiagTypeMismatch && d.NodeID != "" && d.Port != "" {
			found = true
		}
	}
	if !found {
		t.Errorf("no type-mismatch diagnostic with node/port: %v", res.Diagnostics)
	}
}

// TestInvalidParameterIsolation verifies that a single invalid node does not
// fail the whole batch.
func TestInvalidParameterIsolation(t *testing.T) {
	s := newTestSession()
	valid := buildIncrementalGraph()
	invalid := NewGraph()
	invalid.AddNode(&Node{ID: "a", Type: "add", Inputs: map[string]Value{"nonexistent": FloatValue(1)}})
	invalid.AddNode(&Node{ID: "out", Type: OutputNodeType})
	invalid.AddEdge(&Edge{FromNode: "a", FromPort: "out", ToNode: "out", ToPort: "baseColor"})
	invalid.OutputNode = "out"

	results := s.BatchCompile([]*Graph{valid, invalid, valid})
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if !results[0].Success {
		t.Errorf("first graph should succeed: %v", results[0].Diagnostics)
	}
	if results[1].Success {
		t.Error("second graph should fail")
	}
	if !results[2].Success {
		t.Errorf("third graph should succeed: %v", results[2].Diagnostics)
	}
}
