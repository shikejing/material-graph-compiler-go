package compiler

import "github.com/shikejing/material-graph-compiler-go/graph"

// optimize 对图做编译期优化：常量折叠 + 不可达节点消除。
// 返回优化后的图与被消除节点的诊断。对外可观测契约（输出节点/端口）保持不变。
func optimize(g *graph.Graph) (*graph.Graph, []Elimination) {
	// TODO: 实现
	return g, nil
}
