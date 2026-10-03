// Package nodelib 定义节点库：节点定义、端口签名、版本与迁移规则。
package nodelib

import (
	"fmt"
	"sort"

	"github.com/shikejing/material-graph-compiler-go/graph"
)

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
	// Emit 为指定输出端口生成着色表达式（确定性，仅依赖输入表达式与覆盖值）。
	// inputs 包含每个输入端口的表达式（连线变量或常量字面量）。
	Emit func(output graph.PortID, inputs map[graph.PortID]string, overrides map[graph.PortID]*graph.Value) (expr string, err error)
	// Fold 可选：当全部输入都是常量时把节点折叠为常量（仅单输出节点可设置）。
	// 返回 ok=false 表示不可折叠。
	Fold func(constInputs map[graph.PortID]graph.Value) (graph.Value, bool)
}

// Input 按名字查找输入端口定义。
func (d *NodeDef) Input(name graph.PortID) (*PortDef, bool) {
	for i := range d.Inputs {
		if d.Inputs[i].Name == name {
			return &d.Inputs[i], true
		}
	}
	return nil, false
}

// Output 按名字查找输出端口定义。
func (d *NodeDef) Output(name graph.PortID) (*PortDef, bool) {
	for i := range d.Outputs {
		if d.Outputs[i].Name == name {
			return &d.Outputs[i], true
		}
	}
	return nil, false
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

// AddVersion 注册一个新版本（版本号必须严格递增，从 1 开始）。
func (l *Library) AddVersion(v Version) error {
	if v.Number != l.latest+1 {
		return fmt.Errorf("nodelib: version must increase by 1: got %d, want %d", v.Number, l.latest+1)
	}
	if v.Defs == nil {
		v.Defs = map[string]*NodeDef{}
	}
	vv := v
	l.versions[v.Number] = &vv
	l.latest = v.Number
	return nil
}

// Latest 返回最新版本号。
func (l *Library) Latest() int { return l.latest }

// Def 查询指定版本下的节点定义。
func (l *Library) Def(version int, nodeType string) (*NodeDef, bool) {
	v, ok := l.versions[version]
	if !ok {
		return nil, false
	}
	d, ok := v.Defs[nodeType]
	return d, ok
}

// VersionKey 返回指定版本的稳定指纹，用于缓存键。
// 只由版本号与节点定义内容决定，与遍历顺序无关。
func (l *Library) VersionKey(version int) string {
	v, ok := l.versions[version]
	if !ok {
		return fmt.Sprintf("v%d:missing", version)
	}
	names := make([]string, 0, len(v.Defs))
	for name := range v.Defs {
		names = append(names, name)
	}
	sort.Strings(names)
	h := fnvNew()
	fmt.Fprintf(h, "v%d", v.Number)
	for _, name := range names {
		d := v.Defs[name]
		fmt.Fprintf(h, "|%s(in:", name)
		for _, p := range d.Inputs {
			fmt.Fprintf(h, "%s:%s;", p.Name, p.Type)
		}
		fmt.Fprintf(h, ")(out:")
		for _, p := range d.Outputs {
			fmt.Fprintf(h, "%s:%s;", p.Name, p.Type)
		}
		fmt.Fprintf(h, ")")
	}
	return fmt.Sprintf("%x", h.Sum64())
}

// Compatible 判断 from 类型能否隐式转换为 to 类型。
// 规则：同类型恒兼容；标量可扩展为向量/颜色；vec3 可扩展为 color/vec4。
func Compatible(from, to graph.ValueType) bool {
	if from == to {
		return true
	}
	switch from {
	case graph.TypeFloat:
		switch to {
		case graph.TypeVec2, graph.TypeVec3, graph.TypeVec4, graph.TypeColor:
			return true
		}
	case graph.TypeInt:
		if to == graph.TypeFloat {
			return true
		}
	case graph.TypeVec3:
		if to == graph.TypeVec4 || to == graph.TypeColor {
			return true
		}
	}
	return false
}
