package compiler

import (
	"sync"
	"sync/atomic"
	"testing"
)

// TestSingleFlight verifies that concurrent requests for the same graph trigger
// only one compilation.
func TestSingleFlight(t *testing.T) {
	var emitCount int64
	lib := DefaultLibrary()
	lib.Register(&NodeDef{
		Type:    "counter",
		Version: 1,
		Inputs:  []PortDef{{Name: "v", Type: TypeFloat, Default: FloatValue(0)}},
		Outputs: []PortDef{{Name: "out", Type: TypeFloat}},
		Emit: func(c *EmitContext) map[string]string {
			atomic.AddInt64(&emitCount, 1)
			return map[string]string{"out": c.InputExpr("v")}
		},
		// No Eval: not foldable, so Emit is always called.
	})
	s := NewSession(lib, 64, 64)

	g := NewGraph()
	g.AddNode(&Node{ID: "c1", Type: "constant", Inputs: map[string]Value{"value": FloatValue(0.5)}})
	g.AddNode(&Node{ID: "cnt", Type: "counter"})
	g.AddNode(&Node{ID: "out", Type: OutputNodeType})
	g.AddEdge(&Edge{FromNode: "c1", FromPort: "out", ToNode: "cnt", ToPort: "v"})
	g.AddEdge(&Edge{FromNode: "cnt", FromPort: "out", ToNode: "out", ToPort: "baseColor"})
	g.OutputNode = "out"

	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := s.Compile(g)
			if !r.Success {
				t.Errorf("compile failed: %v", r.Diagnostics)
			}
		}()
	}
	wg.Wait()

	if emitCount != 1 {
		t.Errorf("expected 1 compilation (singleflight), got %d", emitCount)
	}
}

// TestLRUEviction verifies that the result cache evicts oldest entries when
// capacity is exceeded.
func TestLRUEviction(t *testing.T) {
	cache := NewResultCache(2)
	p1 := &CompileResult{GraphKey: "a", Success: true}
	p2 := &CompileResult{GraphKey: "b", Success: true}
	p3 := &CompileResult{GraphKey: "c", Success: true}
	cache.Put("a", p1)
	cache.Put("b", p2)
	if cache.Len() != 2 {
		t.Errorf("expected len 2, got %d", cache.Len())
	}
	// Access "a" to make it recently used.
	cache.Get("a")
	// Add "c": "b" should be evicted (oldest), "a" retained.
	cache.Put("c", p3)
	if cache.Len() != 2 {
		t.Errorf("expected len 2 after eviction, got %d", cache.Len())
	}
	if _, ok := cache.Get("b"); ok {
		t.Error("expected 'b' to be evicted")
	}
	if _, ok := cache.Get("a"); !ok {
		t.Error("expected 'a' to be retained")
	}
	if _, ok := cache.Get("c"); !ok {
		t.Error("expected 'c' to be retained")
	}
}

// TestFragmentCacheBounded verifies the fragment cache does not grow beyond
// capacity.
func TestFragmentCacheBounded(t *testing.T) {
	s := NewSession(DefaultLibrary(), 64, 4)
	for i := 0; i < 20; i++ {
		g := NewGraph()
		g.AddNode(&Node{ID: "c1", Type: "constant", Inputs: map[string]Value{"value": FloatValue(float64(i))}})
		g.AddNode(&Node{ID: "out", Type: OutputNodeType})
		g.AddEdge(&Edge{FromNode: "c1", FromPort: "out", ToNode: "out", ToPort: "baseColor"})
		g.OutputNode = "out"
		s.Compile(g)
	}
	if s.frags.Len() > 4 {
		t.Errorf("fragment cache grew beyond capacity: %d", s.frags.Len())
	}
}

// TestNoHalfBakedResults verifies that concurrent compilations never return
// incomplete results.
func TestNoHalfBakedResults(t *testing.T) {
	s := newTestSession()
	g := buildDeterminismGraph()
	const n = 30
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := s.Compile(g)
			if !r.Success {
				t.Errorf("compile failed: %v", r.Diagnostics)
				return
			}
			if r.Program == nil || r.Program.Code == "" {
				t.Error("returned incomplete program")
			}
			if r.GraphKey == "" {
				t.Error("returned empty graph key")
			}
		}()
	}
	wg.Wait()
}

// TestCacheKeyDeterministic verifies the cache key depends only on graph content
// and library version.
func TestCacheKeyDeterministic(t *testing.T) {
	lib := DefaultLibrary()
	g := buildIncrementalGraph()
	k1 := graphKey(g, lib.Version())
	k2 := graphKey(g, lib.Version())
	if k1 != k2 {
		t.Errorf("graph key not deterministic: %s vs %s", k1, k2)
	}
	// Different library version -> different key.
	k3 := graphKey(g, lib.Version()+1)
	if k1 == k3 {
		t.Error("graph key should change with library version")
	}
}
