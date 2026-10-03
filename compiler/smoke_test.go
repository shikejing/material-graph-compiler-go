package compiler

import (
	"fmt"
	"testing"
)

func TestSmoke(t *testing.T) {
	lib := DefaultLibrary()
	g := NewGraph()
	g.AddNode(&Node{ID: "c1", Type: "constant", Inputs: map[string]Value{"value": FloatValue(0.5)}})
	g.AddNode(&Node{ID: "c2", Type: "constant", Inputs: map[string]Value{"value": FloatValue(0.25)}})
	g.AddNode(&Node{ID: "add1", Type: "add"})
	g.AddNode(&Node{ID: "out", Type: OutputNodeType})
	g.AddEdge(&Edge{FromNode: "c1", FromPort: "out", ToNode: "add1", ToPort: "a"})
	g.AddEdge(&Edge{FromNode: "c2", FromPort: "out", ToNode: "add1", ToPort: "b"})
	g.AddEdge(&Edge{FromNode: "add1", FromPort: "out", ToNode: "out", ToPort: "baseColor"})
	g.OutputNode = "out"

	s := NewSession(lib, 16, 16)
	res := s.Compile(g)
	if !res.Success {
		t.Fatalf("compile failed: %v", res.Diagnostics)
	}
	fmt.Println("=== Program ===")
	fmt.Print(res.Program.Code)
	fmt.Println("=== Report ===")
	fmt.Printf("reused: %v\n", res.Report.Reused)
	fmt.Printf("recompiled: %v\n", res.Report.Recompiled)
	fmt.Printf("eliminated: %v\n", res.Report.Eliminated)
	fmt.Printf("folded: %v\n", res.Report.Folded)
}
