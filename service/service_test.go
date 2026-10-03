package service_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/shikejing/material-graph-compiler-go/graph"
	"github.com/shikejing/material-graph-compiler-go/internal/testkit"
	"github.com/shikejing/material-graph-compiler-go/nodelib"
	"github.com/shikejing/material-graph-compiler-go/service"
)

func cyclicGraph() *graph.Graph {
	g := testkit.SampleGraph()
	g.Edges = g.Edges[1:] // 去掉 c1->sum.a
	g.Edges = append(g.Edges, graph.Edge{
		From: graph.PortRef{Node: "prod", Port: "out"},
		To:   graph.PortRef{Node: "sum", Port: "a"},
	})
	return g
}

func TestBatchFailureIsolation(t *testing.T) {
	svc := service.New(testkit.NewLibrary(), 64)
	reqs := []service.Request{
		{ID: "good-1", Graph: testkit.SampleGraph(), LibVersion: 1, Optimize: true},
		{ID: "bad", Graph: cyclicGraph(), LibVersion: 1, Optimize: true},
		{ID: "good-2", Graph: testkit.SampleGraph(), LibVersion: 1},
	}
	results := svc.CompileBatch(reqs)
	if len(results) != 3 {
		t.Fatalf("want 3 results, got %d", len(results))
	}
	// 成功的请求不受影响。
	for _, r := range []service.Result{results[0], results[2]} {
		if r.Err != nil {
			t.Fatalf("request %s should succeed: %v", r.ID, r.Err)
		}
		if r.Program == nil || r.Program.Source == "" {
			t.Fatalf("request %s has empty program", r.ID)
		}
	}
	// 失败的请求带可定位诊断。
	bad := results[1]
	if bad.Err == nil {
		t.Fatal("cyclic graph should fail")
	}
	if bad.Program != nil {
		t.Fatal("failed request must not carry a program")
	}
	foundCycle := false
	for _, d := range bad.Diags {
		if d.Code == "CYCLE" && (d.Node == "sum" || d.Node == "prod") {
			foundCycle = true
		}
	}
	if !foundCycle {
		t.Fatalf("expected CYCLE diagnostic locating sum/prod, got %+v", bad.Diags)
	}
	// 成功与失败的结果都可查询。
	for _, id := range []string{"good-1", "bad", "good-2"} {
		if _, ok := svc.GetResult(id); !ok {
			t.Fatalf("result for %s not queryable", id)
		}
	}
	r, _ := svc.GetResult("bad")
	if r.Err == nil {
		t.Fatal("queried failure result lost its error")
	}
}

func TestConcurrentSameGraphComputedOnce(t *testing.T) {
	svc := service.New(testkit.NewLibrary(), 64)
	const workers = 32
	reqs := make([]service.Request, workers)
	for i := range reqs {
		reqs[i] = service.Request{ID: fmt.Sprintf("r%d", i), Graph: testkit.SampleGraph(), LibVersion: 1, Optimize: true}
	}
	results := svc.CompileBatch(reqs)
	var want string
	cached := 0
	for i, r := range results {
		if r.Err != nil {
			t.Fatalf("request %d failed: %v", i, r.Err)
		}
		if i == 0 {
			want = r.Program.Source
		} else if r.Program.Source != want {
			t.Fatalf("concurrent results diverge at request %d", i)
		}
		if r.FromCache {
			cached++
		}
	}
	// 同一图内容并发请求：至少大部分命中缓存（单飞去重）。
	if cached == 0 {
		t.Fatal("expected cache hits for identical concurrent requests")
	}
	progs, _ := svc.CacheStats()
	if progs != 1 {
		t.Fatalf("identical graphs must share one cache entry, got %d", progs)
	}
}

func TestCacheKeyDependsOnGraphAndLibVersion(t *testing.T) {
	lib := testkit.NewLibrary()
	svc := service.New(lib, 64)
	g := testkit.SampleGraph()
	r1 := svc.Compile(service.Request{ID: "a", Graph: g, LibVersion: 1})
	if r1.Err != nil {
		t.Fatal(r1.Err)
	}
	r2 := svc.Compile(service.Request{ID: "b", Graph: g, LibVersion: 1})
	if !r2.FromCache {
		t.Fatal("same graph + same version should hit cache")
	}
	// v1 编写的图直接按 v2 编译会被拒绝（端口已改名），必须先迁移。
	r3 := svc.Compile(service.Request{ID: "c", Graph: g, LibVersion: 2})
	if r3.Err == nil {
		t.Fatal("v1 graph compiled against v2 must be rejected")
	}
	migrated, err := nodelib.MigrateGraph(g, lib, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	r3 = svc.Compile(service.Request{ID: "c2", Graph: migrated, LibVersion: 2})
	if r3.Err != nil {
		t.Fatalf("migrated graph should compile under v2: %v", r3.Err)
	}
	if r3.FromCache {
		t.Fatal("different library version must not hit the same cache entry")
	}
	g2 := testkit.SampleGraph()
	v := graph.FloatValue(42)
	g2.Nodes[0].Overrides["value"] = &v
	r4 := svc.Compile(service.Request{ID: "d", Graph: g2, LibVersion: 1})
	if r4.FromCache {
		t.Fatal("changed graph must not hit the old cache entry")
	}
	progs, _ := svc.CacheStats()
	if progs != 3 {
		t.Fatalf("expected 3 distinct cache entries, got %d", progs)
	}
}

func TestIncrementalViaService(t *testing.T) {
	svc := service.New(testkit.NewLibrary(), 64)
	g := testkit.SampleGraph()
	first := svc.Compile(service.Request{ID: "first", Graph: g, LibVersion: 1})
	if first.Err != nil {
		t.Fatal(first.Err)
	}
	// 局部改动一个常量。
	g2 := testkit.SampleGraph()
	v := graph.FloatValue(10)
	g2.Nodes[0].Overrides["value"] = &v
	second := svc.Compile(service.Request{ID: "second", Graph: g2, LibVersion: 1})
	if second.Err != nil {
		t.Fatal(second.Err)
	}
	// 节点级复用发生：未受影响的节点被复用。
	if len(second.Reused) == 0 {
		t.Fatalf("expected some nodes reused, rebuilt all: %v", second.Rebuilt)
	}
	// 增量结果与全新服务的全量结果逐字节一致。
	fresh := service.New(testkit.NewLibrary(), 64)
	full := fresh.Compile(service.Request{ID: "full", Graph: g2, LibVersion: 1})
	if full.Err != nil {
		t.Fatal(full.Err)
	}
	if second.Program.Source != full.Program.Source {
		t.Fatalf("incremental != full:\n%s\nvs\n%s", second.Program.Source, full.Program.Source)
	}
}

func TestCacheBoundedUnderLoad(t *testing.T) {
	svc := service.New(testkit.NewLibrary(), 32)
	// 大量不同的图：缓存占用必须有界。
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			g := testkit.SampleGraph()
			v := graph.FloatValue(float64(i))
			g.Nodes[0].Overrides["value"] = &v
			svc.Compile(service.Request{ID: fmt.Sprintf("load-%d", i), Graph: g, LibVersion: 1})
		}(i)
	}
	wg.Wait()
	progs, lines := svc.CacheStats()
	if progs > 32 {
		t.Fatalf("program cache unbounded: %d", progs)
	}
	if lines > 32*16 {
		t.Fatalf("line cache unbounded: %d", lines)
	}
}

func TestEmitErrorIsolated(t *testing.T) {
	// 节点参数非法（覆盖值类型不匹配）只影响该请求。
	svc := service.New(testkit.NewLibrary(), 64)
	g := testkit.SampleGraph()
	bad := graph.VecValue(graph.TypeVec2, 1, 2) // vec2 不能赋给 float 端口
	g.Nodes[0].Overrides["value"] = &bad
	res := svc.Compile(service.Request{ID: "bad-override", Graph: g, LibVersion: 1})
	if res.Err == nil {
		t.Fatal("expected type mismatch error")
	}
	found := false
	for _, d := range res.Diags {
		if d.Code == "TYPE_MISMATCH" && d.Node == "c1" && d.Port == "value" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected TYPE_MISMATCH at c1.value, got %+v", res.Diags)
	}
	// 同服务其他请求不受影响。
	ok := svc.Compile(service.Request{ID: "ok", Graph: testkit.SampleGraph(), LibVersion: 1})
	if ok.Err != nil {
		t.Fatalf("unrelated request failed: %v", ok.Err)
	}
	if !strings.Contains(ok.Program.Source, "output = v_mat_out;") {
		t.Fatal("valid request output malformed")
	}
}
