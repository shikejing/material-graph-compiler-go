package compiler

import "testing"

// buildIncrementalGraph builds a graph for incremental testing.
func buildIncrementalGraph() *Graph {
	g := NewGraph()
	g.AddNode(&Node{ID: "c1", Type: "constant", Inputs: map[string]Value{"value": FloatValue(0.5)}})
	g.AddNode(&Node{ID: "c2", Type: "constant", Inputs: map[string]Value{"value": FloatValue(0.25)}})
	g.AddNode(&Node{ID: "add1", Type: "add"})
	g.AddNode(&Node{ID: "out", Type: OutputNodeType})
	g.AddEdge(&Edge{FromNode: "c1", FromPort: "out", ToNode: "add1", ToPort: "a"})
	g.AddEdge(&Edge{FromNode: "c2", FromPort: "out", ToNode: "add1", ToPort: "b"})
	g.AddEdge(&Edge{FromNode: "add1", FromPort: "out", ToNode: "out", ToPort: "baseColor"})
	g.OutputNode = "out"
	return g
}

// TestIncrementalConsistency verifies that incremental recompilation produces
// byte-identical output to a full recompile, and that reused/recompiled nodes
// are reported.
func TestIncrementalConsistency(t *testing.T) {
	lib := DefaultLibrary()

	// First compilation (full).
	s1 := NewSession(lib, 64, 64)
	g1 := buildIncrementalGraph()
	r1 := s1.Compile(g1)
	if !r1.Success {
		t.Fatalf("first compile failed: %v", r1.Diagnostics)
	}

	// Modify the graph: change c1's value.
	g2 := buildIncrementalGraph()
	g2.Nodes["c1"].Inputs["value"] = FloatValue(0.75)

	// Incremental compilation (same session, reuses fragments).
	r2 := s1.Compile(g2)
	if !r2.Success {
		t.Fatalf("incremental compile failed: %v", r2.Diagnostics)
	}

	// Full recompile (new session, no cache).
	s2 := NewSession(lib, 64, 64)
	r3 := s2.Compile(g2)
	if !r3.Success {
		t.Fatalf("full recompile failed: %v", r3.Diagnostics)
	}

	// Incremental output must match full recompile byte-for-byte.
	if r2.Program.Code != r3.Program.Code {
		t.Errorf("incremental output differs from full recompile:\n--- incremental ---\n%s\n--- full ---\n%s", r2.Program.Code, r3.Program.Code)
	}

	// The changed node (c1) and its downstream (add1, out) should be recompiled;
	// c2 should be reused.
	if !contains(r2.Report.Recompiled, "c1") {
		t.Errorf("c1 should be recompiled, recompiled: %v", r2.Report.Recompiled)
	}
	if !contains(r2.Report.Reused, "c2") {
		t.Errorf("c2 should be reused, reused: %v", r2.Report.Reused)
	}
	// add1 depends on c1, so it should be recompiled.
	if !contains(r2.Report.Recompiled, "add1") {
		t.Errorf("add1 should be recompiled (downstream of c1), recompiled: %v", r2.Report.Recompiled)
	}
}

// TestIncrementalNodeAdded verifies that adding a node recompiles only affected
// parts.
func TestIncrementalNodeAdded(t *testing.T) {
	lib := DefaultLibrary()
	s1 := NewSession(lib, 64, 64)
	g1 := buildIncrementalGraph()
	r1 := s1.Compile(g1)
	if !r1.Success {
		t.Fatalf("first compile failed: %v", r1.Diagnostics)
	}

	// Add a new constant node and multiply.
	g2 := buildIncrementalGraph()
	g2.AddNode(&Node{ID: "c3", Type: "constant", Inputs: map[string]Value{"value": FloatValue(2)}})
	g2.AddNode(&Node{ID: "mul1", Type: "multiply"})
	// Rewire: add1.out -> mul1.a, c3.out -> mul1.b, mul1.out -> out.baseColor
	g2.Edges = nil
	g2.AddEdge(&Edge{FromNode: "c1", FromPort: "out", ToNode: "add1", ToPort: "a"})
	g2.AddEdge(&Edge{FromNode: "c2", FromPort: "out", ToNode: "add1", ToPort: "b"})
	g2.AddEdge(&Edge{FromNode: "add1", FromPort: "out", ToNode: "mul1", ToPort: "a"})
	g2.AddEdge(&Edge{FromNode: "c3", FromPort: "out", ToNode: "mul1", ToPort: "b"})
	g2.AddEdge(&Edge{FromNode: "mul1", FromPort: "out", ToNode: "out", ToPort: "baseColor"})

	r2 := s1.Compile(g2)
	if !r2.Success {
		t.Fatalf("incremental compile failed: %v", r2.Diagnostics)
	}

	s2 := NewSession(lib, 64, 64)
	r3 := s2.Compile(g2)
	if !r3.Success {
		t.Fatalf("full recompile failed: %v", r3.Diagnostics)
	}

	if r2.Program.Code != r3.Program.Code {
		t.Errorf("incremental output differs from full recompile")
	}
	// c1, c2, add1 should be reused (unchanged); c3, mul1, out recompiled.
	if !contains(r2.Report.Reused, "c1") {
		t.Errorf("c1 should be reused, reused: %v", r2.Report.Reused)
	}
	if !contains(r2.Report.Reused, "c2") {
		t.Errorf("c2 should be reused, reused: %v", r2.Report.Reused)
	}
}

// TestIncrementalNodeRemoved verifies that removing a node recompiles correctly.
func TestIncrementalNodeRemoved(t *testing.T) {
	lib := DefaultLibrary()
	s1 := NewSession(lib, 64, 64)
	g1 := buildIncrementalGraph()
	r1 := s1.Compile(g1)
	if !r1.Success {
		t.Fatalf("first compile failed: %v", r1.Diagnostics)
	}

	// Remove c2 and its edge.
	g2 := buildIncrementalGraph()
	delete(g2.Nodes, "c2")
	g2.Edges = nil
	g2.AddEdge(&Edge{FromNode: "c1", FromPort: "out", ToNode: "add1", ToPort: "a"})
	g2.AddEdge(&Edge{FromNode: "add1", FromPort: "out", ToNode: "out", ToPort: "baseColor"})

	r2 := s1.Compile(g2)
	if !r2.Success {
		t.Fatalf("incremental compile failed: %v", r2.Diagnostics)
	}

	s2 := NewSession(lib, 64, 64)
	r3 := s2.Compile(g2)
	if !r3.Success {
		t.Fatalf("full recompile failed: %v", r3.Diagnostics)
	}

	if r2.Program.Code != r3.Program.Code {
		t.Errorf("incremental output differs from full recompile")
	}
	// c1 should be reused; add1 and out recompiled.
	if !contains(r2.Report.Reused, "c1") {
		t.Errorf("c1 should be reused, reused: %v", r2.Report.Reused)
	}
}
