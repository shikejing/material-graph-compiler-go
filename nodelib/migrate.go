package nodelib

import "github.com/shikejing/material-graph-compiler-go/graph"

// MigrateError 表示旧图无法自动迁移到新库版本。
type MigrateError struct {
	Reasons []string
}

func (e *MigrateError) Error() string { return "graph migration failed" }

// MigrateGraph 把针对 fromVersion 编写的图迁移到 toVersion。
// 返回迁移后的新图；无法自动兼容时返回 *MigrateError，原图不被修改。
func MigrateGraph(g *graph.Graph, lib *Library, fromVersion, toVersion int) (*graph.Graph, error) {
	// TODO: 实现
	return nil, nil
}
