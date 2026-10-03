package service

import (
	"github.com/shikejing/material-graph-compiler-go/compiler"
	"github.com/shikejing/material-graph-compiler-go/graph"
	"github.com/shikejing/material-graph-compiler-go/nodelib"
)

// incrementalState 记录一张图上次编译的逐节点结果，用于增量重编。
type incrementalState struct {
	nodeHashes map[graph.NodeID]string
	program    *compiler.Program
}

// compileIncremental 只重编受影响的节点，复用未变化结果。
// 产出与全量编译逐字节一致。
func compileIncremental(g *graph.Graph, lib *nodelib.Library, version int, prev *incrementalState) (*compiler.Program, *incrementalState, []graph.NodeID, []graph.NodeID, error) {
	// TODO: 实现
	return nil, nil, nil, nil, nil
}
