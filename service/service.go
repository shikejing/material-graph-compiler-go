// Package service 提供面向渲染侧的编译服务：批量请求、失败隔离、增量重编与缓存。
package service

import (
	"github.com/shikejing/material-graph-compiler-go/cache"
	"github.com/shikejing/material-graph-compiler-go/compiler"
	"github.com/shikejing/material-graph-compiler-go/graph"
	"github.com/shikejing/material-graph-compiler-go/nodelib"
)

// Request 是一次编译请求。
type Request struct {
	ID         string
	Graph      *graph.Graph
	LibVersion int // 0 表示使用节点库最新版本
}

// Result 是一次编译的结果（成功或失败二选一）。
type Result struct {
	ID       string
	Program  *compiler.Program
	Diags    []graph.Diagnostic
	Err      error
	Reused   []graph.NodeID // 增量重编中被复用的节点
	Rebuilt  []graph.NodeID // 增量重编中被重编的节点
	FromCache bool
}

// Service 管理编译请求、缓存与增量状态。
type Service struct {
	lib   *nodelib.Library
	cache *cache.Cache
}

// New 创建编译服务。
func New(lib *nodelib.Library, cacheCapacity int) *Service {
	// TODO: 实现
	return nil
}

// Compile 编译单个请求。
func (s *Service) Compile(req Request) Result {
	// TODO: 实现
	return Result{ID: req.ID}
}

// CompileBatch 并发编译一批请求，单请求失败被隔离，不影响其他请求。
func (s *Service) CompileBatch(reqs []Request) []Result {
	// TODO: 实现
	return nil
}
