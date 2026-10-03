// Package nodelib 定义节点库：节点定义、端口签名、版本与迁移规则。
package nodelib

import "github.com/shikejing/material-graph-compiler-go/graph"

// PortDef 描述节点定义中的一个端口。
type PortDef struct {
	Name    graph.PortID
	Type    graph.ValueType
	Default *graph.Value // 输入端口的默认值；输出端口为 nil
}

// NodeDef 是一类节点的定义。
type NodeDef struct {
	Type    string
	Inputs  []PortDef
	Outputs []PortDef
	// Emit 为该节点生成着色代码片段（确定性，仅依赖输入表达式与覆盖值）。
	Emit func(inputs map[graph.PortID]string, overrides map[graph.PortID]*graph.Value) (expr string, err error)
}

// Migration 描述某节点类型从旧版本升级到新版本时的适配规则。
type Migration struct {
	Type         string
	FromVersion  int
	RenamedPorts map[graph.PortID]graph.PortID // 旧端口名 -> 新端口名
	RemovedPorts map[graph.PortID]graph.Value  // 被删除端口及其替代常量
	// NewDefaults 为新增端口提供默认值；新增端口若无默认值则无法自动迁移。
	NewDefaults map[graph.PortID]graph.Value
}

// Version 是节点库的一个版本快照。
type Version struct {
	Number     int
	Defs       map[string]*NodeDef
	Migrations []Migration // 从 Number-1 升级到 Number 的规则
}

// Library 是版本化的节点库。
type Library struct {
	versions map[int]*Version
	latest   int
}

// NewLibrary 创建空节点库。
func NewLibrary() *Library { return &Library{versions: map[int]*Version{}} }

// AddVersion 注册一个新版本（版本号必须严格递增）。
func (l *Library) AddVersion(v Version) error {
	// TODO: 实现
	return nil
}

// Latest 返回最新版本号。
func (l *Library) Latest() int { return l.latest }

// Def 查询指定版本下的节点定义。
func (l *Library) Def(version int, nodeType string) (*NodeDef, bool) {
	// TODO: 实现
	return nil, false
}

// Compatible 判断 from 类型能否隐式转换为 to 类型。
func Compatible(from, to graph.ValueType) bool {
	// TODO: 实现
	return false
}
