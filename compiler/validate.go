package compiler

import (
	"github.com/shikejing/material-graph-compiler-go/graph"
	"github.com/shikejing/material-graph-compiler-go/nodelib"
)

// Validate 对图做整体校验：端口存在性、连线类型兼容、环检测。
// 任一失败都会返回 *graph.Error，调用方不得产出任何部分结果。
func Validate(g *graph.Graph, lib *nodelib.Library, version int) *graph.Error {
	// TODO: 实现
	return nil
}
