// Package compiler 把校验通过的材质图编译为确定性的着色程序。
package compiler

import (
	"github.com/shikejing/material-graph-compiler-go/graph"
	"github.com/shikejing/material-graph-compiler-go/nodelib"
)

// Program 是编译产物：确定性的着色程序文本。
type Program struct {
	Source string
	// Eliminated 记录被优化消除的节点及原因。
	Eliminated []Elimination
}

// Elimination 说明一个节点为何被消除。
type Elimination struct {
	Node   graph.NodeID
	Reason string
}

// Options 控制编译行为。
type Options struct {
	Optimize bool // 是否启用常量折叠与不可达消除
}

// Compile 编译材质图。图非法时返回 *graph.Error，不产出任何部分结果。
func Compile(g *graph.Graph, lib *nodelib.Library, version int, opts Options) (*Program, error) {
	// TODO: 实现
	return nil, nil
}
