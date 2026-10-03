// Package service 提供面向渲染侧的编译服务：批量请求、失败隔离、增量重编与结果缓存。
//
// 语义：
//   - 编译全程在本地进程内完成，不依赖任何外部服务；
//   - 同一图内容 + 同一节点库版本的并发请求只计算一次（缓存单飞）；
//   - 缓存键只由图内容与节点库版本决定；
//   - 局部改动通过节点级行缓存实现增量重编，产出与全量编译逐字节一致；
//   - 单个请求失败被隔离，不影响同批其他请求；成功与失败结果均可查询。
package service

import (
	"fmt"
	"sync"

	"github.com/shikejing/material-graph-compiler-go/cache"
	"github.com/shikejing/material-graph-compiler-go/compiler"
	"github.com/shikejing/material-graph-compiler-go/graph"
	"github.com/shikejing/material-graph-compiler-go/nodelib"
)

// Request 是一次编译请求。
type Request struct {
	ID         string
	Graph      *graph.Graph
	LibVersion int  // 0 表示使用节点库最新版本
	Optimize   bool // 是否启用编译期优化
}

// Result 是一次编译的结果（成功或失败二选一）。
type Result struct {
	ID        string
	Program   *compiler.Program
	Diags     []graph.Diagnostic
	Err       error
	Reused    []graph.NodeID // 增量重编中被复用的节点
	Rebuilt   []graph.NodeID // 增量重编中被重编的节点
	FromCache bool           // 整体程序是否直接命中缓存
}

// compiledEntry 是程序缓存的值。
type compiledEntry struct {
	prog    *compiler.Program
	reused  []graph.NodeID
	rebuilt []graph.NodeID
}

// Service 管理编译请求、缓存与增量状态。
type Service struct {
	lib       *nodelib.Library
	progCache *cache.Cache[compiledEntry]
	lineCache *cache.Cache[string]

	mu      sync.Mutex
	results map[string]Result
}

// New 创建编译服务。cacheCapacity 为程序缓存与节点行缓存的容量上限。
func New(lib *nodelib.Library, cacheCapacity int) *Service {
	return &Service{
		lib:       lib,
		progCache: cache.New[compiledEntry](cacheCapacity),
		lineCache: cache.New[string](cacheCapacity * 16),
		results:   make(map[string]Result),
	}
}

// Compile 编译单个请求。错误被捕获并放入 Result，不会 panic 传播。
func (s *Service) Compile(req Request) (res Result) {
	res.ID = req.ID
	defer func() {
		if r := recover(); r != nil {
			res = Result{ID: req.ID, Err: fmt.Errorf("compile panicked: %v", r)}
		}
		s.store(res)
	}()

	version := req.LibVersion
	if version == 0 {
		version = s.lib.Latest()
	}
	opts := compiler.Options{Optimize: req.Optimize}
	key := compiler.CacheKey(req.Graph, s.lib, version, opts)

	if ent, ok := s.progCache.Peek(key); ok {
		res.Program = ent.prog
		res.Reused = ent.reused
		res.Rebuilt = ent.rebuilt
		res.FromCache = true
		return res
	}

	ent, err := s.progCache.GetOrCompute(key, func() (compiledEntry, error) {
		ir, err := compiler.CompileIncremental(req.Graph, s.lib, version, opts, s.lineCache)
		if err != nil {
			return compiledEntry{}, err
		}
		return compiledEntry{prog: ir.Program, reused: ir.Reused, rebuilt: ir.Rebuilt}, nil
	})
	if err != nil {
		res.Err = err
		if verr, ok := err.(*graph.Error); ok {
			res.Diags = verr.Diags
		}
		return res
	}
	res.Program = ent.prog
	res.Reused = ent.reused
	res.Rebuilt = ent.rebuilt
	return res
}

// CompileBatch 并发编译一批请求；单请求失败被隔离，不影响其他请求。
// 返回与请求一一对应的结果（顺序与请求一致）。
func (s *Service) CompileBatch(reqs []Request) []Result {
	results := make([]Result, len(reqs))
	var wg sync.WaitGroup
	for i, req := range reqs {
		wg.Add(1)
		go func(i int, req Request) {
			defer wg.Done()
			results[i] = s.Compile(req)
		}(i, req)
	}
	wg.Wait()
	return results
}

// GetResult 查询某次请求的结果（成功与失败都可查询）。
func (s *Service) GetResult(id string) (Result, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.results[id]
	return r, ok
}

// CacheStats 返回缓存占用（程序缓存条目数、节点行缓存条目数）。
func (s *Service) CacheStats() (programs, lines int) {
	return s.progCache.Len(), s.lineCache.Len()
}

func (s *Service) store(r Result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results[r.ID] = r
}
