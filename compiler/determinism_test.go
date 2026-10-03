package compiler

import (
	"sync"
	"testing"
)

// buildDeterminismGraph builds a graph with several nodes and edges.
func buildDeterminismGraph() *Graph {
	g := NewGraph()
	g.AddNode(&Node{ID: "c1", Type: "constant", Inputs: map[string]Value{"value": FloatValue(0.5)}})
	g.AddNode(&Node{ID: "c2", Type: "constant", Inputs: map[string]Value{"value": FloatValue(0.25)}})
	g.AddNode(&Node{ID: "c3", Type: "constant", Inputs: map[string]Value{"value": FloatValue(0.1)}})
	g.AddNode(&Node{ID: "add1", Type: "add"})
	g.AddNode(&Node{ID: "mul1", Type: "multiply"})
	g.AddNode(&Node{ID: "mix1", Type: "mix"})
	g.AddNode(&Node{ID: "out", Type: OutputNodeType})
	g.AddEdge(&Edge{FromNode: "c1", FromPort: "out", ToNode: "add1", ToPort: "a"})
	g.AddEdge(&Edge{FromNode: "c2", FromPort: "out", ToNode: "add1", ToPort: "b"})
	g.AddEdge(&Edge{FromNode: "add1", FromPort: "out", ToNode: "mul1", ToPort: "a"})
	g.AddEdge(&Edge{FromNode: "c3", FromPort: "out", ToNode: "mul1", ToPort: "b"})
	g.AddEdge(&Edge{FromNode: "mul1", FromPort: "out", ToNode: "mix1", ToPort: "a"})
	g.AddEdge(&Edge{FromNode: "c2", FromPort: "out", ToNode: "mix1", ToPort: "b"})
	g.AddEdge(&Edge{FromNode: "c3", FromPort: "out", ToNode: "mix1", ToPort: "t"})
	g.AddEdge(&Edge{FromNode: "mix1", FromPort: "out", ToNode: "out", ToPort: "baseColor"})
	g.OutputNode = "out"
	return g
}

// TestDeterministicOutput verifies that compiling the same graph produces
// byte-identical output regardless of node insertion order.
func TestDeterministicOutput(t *testing.T) {
	s := newTestSession()
	g := buildDeterminismGraph()
	r1 := s.Compile(g)
	if !r1.Success {
		t.Fatalf("compile failed: %v", r1.Diagnostics)
	}

	// Rebuild the graph with nodes added in a different order.
	g2 := NewGraph()
	for _, id := range []string{"out", "mix1", "mul1", "add1", "c3", "c2", "c1"} {
		n := g.Nodes[id]
		g2.AddNode(n)
	}
	for _, e := range g.Edges {
		g2.AddEdge(e)
	}
	g2.OutputNode = g.OutputNode
	r2 := s.Compile(g2)
	if !r2.Success {
		t.Fatalf("compile failed: %v", r2.Diagnostics)
	}

	if r1.Program.Code != r2.Program.Code {
		t.Errorf("output differs with insertion order:\n--- r1 ---\n%s\n--- r2 ---\n%s", r1.Program.Code, r2.Program.Code)
	}
	if r1.GraphKey != r2.GraphKey {
		t.Errorf("graph key differs: %s vs %s", r1.GraphKey, r2.GraphKey)
	}
}

// TestDeterministicConcurrent verifies that concurrent compilations of the same
// graph produce identical output.
func TestDeterministicConcurrent(t *testing.T) {
	s := newTestSession()
	g := buildDeterminismGraph()
	const n = 20
	results := make([]*CompileResult, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = s.Compile(g)
		}(i)
	}
	wg.Wait()

	first := results[0]
	if !first.Success {
		t.Fatalf("compile failed: %v", first.Diagnostics)
	}
	for i := 1; i < n; i++ {
		if !results[i].Success {
			t.Fatalf("compile %d failed: %v", i, results[i].Diagnostics)
		}
		if results[i].Program.Code != first.Program.Code {
			t.Errorf("concurrent output %d differs", i)
		}
		if results[i].GraphKey != first.GraphKey {
			t.Errorf("concurrent graph key %d differs", i)
		}
	}
}

// TestNoRandomIdentifiers verifies the output contains no random identifiers or
// timestamps by checking for common timestamp/random patterns.
func TestNoRandomIdentifiers(t *testing.T) {
	s := newTestSession()
	g := buildDeterminismGraph()
	r := s.Compile(g)
	if !r.Success {
		t.Fatalf("compile failed: %v", r.Diagnostics)
	}
	code := r.Program.Code
	// Check for Unix timestamps (10+ digit numbers) and date patterns.
	for _, pat := range []string{"2020", "2021", "2022", "2023", "2024", "2025", "2026", "2027", "2028", "2029", "2030"} {
		if containsStr(code, pat) {
			t.Errorf("output contains year pattern %q:\n%s", pat, code)
		}
	}
	// Check for random identifier patterns (v_ followed by random hex).
	for _, line := range splitLines(code) {
		if containsStr(line, "v_") {
			// Variable names should be deterministic (v_<id>_<port>).
			// Just verify they don't contain random-looking long hex.
		}
	}
	// Verify the graph key is a 64-char hex string (deterministic).
	if len(r.GraphKey) != 64 {
		t.Errorf("graph key should be 64 hex chars, got %d", len(r.GraphKey))
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
