package compiler

import "testing"

// TestConstantFolding verifies that constant nodes are folded into literals.
func TestConstantFolding(t *testing.T) {
	s := newTestSession()
	g := NewGraph()
	g.AddNode(&Node{ID: "c1", Type: "constant", Inputs: map[string]Value{"value": FloatValue(0.5)}})
	g.AddNode(&Node{ID: "c2", Type: "constant", Inputs: map[string]Value{"value": FloatValue(0.25)}})
	g.AddNode(&Node{ID: "add1", Type: "add"})
	g.AddNode(&Node{ID: "out", Type: OutputNodeType})
	g.AddEdge(&Edge{FromNode: "c1", FromPort: "out", ToNode: "add1", ToPort: "a"})
	g.AddEdge(&Edge{FromNode: "c2", FromPort: "out", ToNode: "add1", ToPort: "b"})
	g.AddEdge(&Edge{FromNode: "add1", FromPort: "out", ToNode: "out", ToPort: "baseColor"})
	g.OutputNode = "out"

	res := s.Compile(g)
	if !res.Success {
		t.Fatalf("compile failed: %v", res.Diagnostics)
	}
	// add1 should be folded (0.5 + 0.25 = 0.75).
	if !contains(res.Report.Folded, "add1") {
		t.Errorf("expected add1 to be folded, folded: %v", res.Report.Folded)
	}
	// The program should contain the literal 0.75, not a variable reference.
	if !containsStr(res.Program.Code, "0.75") {
		t.Errorf("expected literal 0.75 in code:\n%s", res.Program.Code)
	}
	if containsStr(res.Program.Code, "v_add1") {
		t.Errorf("folded node should not emit a variable:\n%s", res.Program.Code)
	}
}

// TestDeadCodeElimination verifies that unreachable nodes are eliminated.
func TestDeadCodeElimination(t *testing.T) {
	s := newTestSession()
	g := NewGraph()
	g.AddNode(&Node{ID: "dead", Type: "constant", Inputs: map[string]Value{"value": FloatValue(99)}})
	g.AddNode(&Node{ID: "live", Type: "constant", Inputs: map[string]Value{"value": FloatValue(0.5)}})
	g.AddNode(&Node{ID: "out", Type: OutputNodeType})
	g.AddEdge(&Edge{FromNode: "live", FromPort: "out", ToNode: "out", ToPort: "baseColor"})
	g.OutputNode = "out"

	res := s.Compile(g)
	if !res.Success {
		t.Fatalf("compile failed: %v", res.Diagnostics)
	}
	if !contains(res.Report.Eliminated, "dead") {
		t.Errorf("expected dead node to be eliminated, eliminated: %v", res.Report.Eliminated)
	}
	if containsStr(res.Program.Code, "v_dead") {
		t.Errorf("eliminated node should not appear in code:\n%s", res.Program.Code)
	}
	// The eliminated node should have a diagnostic explaining why.
	found := false
	for _, d := range res.Diagnostics {
		if d.Kind == DiagEliminated && d.NodeID == "dead" {
			found = true
		}
	}
	if !found {
		t.Errorf("no eliminated diagnostic for dead node: %v", res.Diagnostics)
	}
}

// TestFoldPreservesSemantics verifies that folding produces the same result as
// non-folded compilation by comparing the literal value.
func TestFoldPreservesSemantics(t *testing.T) {
	s := newTestSession()
	g := NewGraph()
	g.AddNode(&Node{ID: "c1", Type: "constant", Inputs: map[string]Value{"value": FloatValue(2)}})
	g.AddNode(&Node{ID: "c2", Type: "constant", Inputs: map[string]Value{"value": FloatValue(3)}})
	g.AddNode(&Node{ID: "mul", Type: "multiply"})
	g.AddNode(&Node{ID: "out", Type: OutputNodeType})
	g.AddEdge(&Edge{FromNode: "c1", FromPort: "out", ToNode: "mul", ToPort: "a"})
	g.AddEdge(&Edge{FromNode: "c2", FromPort: "out", ToNode: "mul", ToPort: "b"})
	g.AddEdge(&Edge{FromNode: "mul", FromPort: "out", ToNode: "out", ToPort: "baseColor"})
	g.OutputNode = "out"

	res := s.Compile(g)
	if !res.Success {
		t.Fatalf("compile failed: %v", res.Diagnostics)
	}
	// 2 * 3 = 6.
	if !containsStr(res.Program.Code, "6") {
		t.Errorf("expected folded result 6 in code:\n%s", res.Program.Code)
	}
}

// contains checks if a string is in a slice.
func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// containsStr checks if a string contains a substring.
func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
