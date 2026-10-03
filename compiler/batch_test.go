package compiler

import "testing"

// TestBatchCompilePartialFailure verifies that a single graph failure does not
// fail the whole batch, and that successes and failures are distinguishable.
func TestBatchCompilePartialFailure(t *testing.T) {
	s := newTestSession()

	valid := buildIncrementalGraph()
	invalid := NewGraph()
	invalid.AddNode(&Node{ID: "a", Type: "add"})
	invalid.AddNode(&Node{ID: "b", Type: "multiply"})
	invalid.AddNode(&Node{ID: "out", Type: OutputNodeType})
	invalid.AddEdge(&Edge{FromNode: "a", FromPort: "out", ToNode: "b", ToPort: "a"})
	invalid.AddEdge(&Edge{FromNode: "b", FromPort: "out", ToNode: "a", ToPort: "a"}) // cycle
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
		t.Error("second graph should fail (cycle)")
	}
	if !results[2].Success {
		t.Errorf("third graph should succeed: %v", results[2].Diagnostics)
	}
	// The failure should have a cycle diagnostic.
	found := false
	for _, d := range results[1].Diagnostics {
		if d.Kind == DiagCycle {
			found = true
		}
	}
	if !found {
		t.Errorf("expected cycle diagnostic: %v", results[1].Diagnostics)
	}
}

// TestBatchCompileAllSuccess verifies a batch of valid graphs all succeed.
func TestBatchCompileAllSuccess(t *testing.T) {
	s := newTestSession()
	graphs := []*Graph{buildIncrementalGraph(), buildDeterminismGraph()}
	results := s.BatchCompile(graphs)
	for i, r := range results {
		if !r.Success {
			t.Errorf("graph %d should succeed: %v", i, r.Diagnostics)
		}
	}
}

// TestBatchCompileAllFailure verifies a batch of invalid graphs all fail with
// diagnostics.
func TestBatchCompileAllFailure(t *testing.T) {
	s := newTestSession()
	invalid := NewGraph()
	invalid.AddNode(&Node{ID: "a", Type: "add"})
	// No output node.
	results := s.BatchCompile([]*Graph{invalid})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("graph should fail (no output)")
	}
	if !HasError(results[0].Diagnostics) {
		t.Error("expected error diagnostics")
	}
}
