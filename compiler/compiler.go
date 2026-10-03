// Package compiler 把校验通过的材质图编译为确定性的着色程序。
//
// 确定性保证：同一张图在任意遍历顺序与并发度下产出逐字节一致的 Source。
// 手段：拓扑排序使用字典序决胜、所有集合迭代前排序、变量名只由节点 ID 派生、
// 输出不含时间戳与随机标识。
package compiler

import (
	"fmt"
	"sort"
	"strings"

	"github.com/shikejing/material-graph-compiler-go/graph"
	"github.com/shikejing/material-graph-compiler-go/nodelib"
)

// Program 是编译产物：确定性的着色程序文本。
type Program struct {
	Source string
	// Eliminated 记录被优化消除的节点及原因。
	Eliminated []Elimination
	// Lines 是逐节点的代码行（节点 ID -> 行），供增量复用。
	Lines map[graph.NodeID]string
	// Order 是行在 Source 中的拓扑顺序。
	Order []graph.NodeID
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

// CacheKey 返回编译缓存键：只由图内容与节点库版本决定。
func CacheKey(g *graph.Graph, lib *nodelib.Library, version int, opts Options) string {
	return fmt.Sprintf("g=%s;lib=%s;opt=%v", g.Key(), lib.VersionKey(version), opts.Optimize)
}

// Compile 编译材质图。图非法时返回 *graph.Error，不产出任何部分结果。
func Compile(g *graph.Graph, lib *nodelib.Library, version int, opts Options) (*Program, error) {
	if verr := Validate(g, lib, version); verr != nil {
		return nil, verr
	}
	work := g
	var elims []Elimination
	if opts.Optimize {
		work, elims = optimize(g, lib, version)
	}
	order, lines, err := emitAll(work, lib, version)
	if err != nil {
		return nil, err
	}
	var sb strings.Builder
	sb.WriteString("// material program\n")
	fmt.Fprintf(&sb, "// library %s\n", lib.VersionKey(version))
	for _, id := range order {
		sb.WriteString(lines[id])
		sb.WriteByte('\n')
	}
	fmt.Fprintf(&sb, "output = %s;\n", varName(g.OutputNode, g.OutputPort))
	return &Program{Source: sb.String(), Eliminated: elims, Lines: lines, Order: order}, nil
}

// emitAll 按确定拓扑顺序为每个节点的每个被使用的输出端口生成一行代码。
func emitAll(g *graph.Graph, lib *nodelib.Library, version int) ([]graph.NodeID, map[graph.NodeID]string, error) {
	order, err := topoSort(g)
	if err != nil {
		return nil, nil, err
	}
	lines := make(map[graph.NodeID]string, len(order))
	for _, id := range order {
		n := findNode(g, id)
		def, _ := lib.Def(version, n.Type)
		line, err := emitNode(g, *n, def, lib, version)
		if err != nil {
			return nil, nil, err
		}
		lines[id] = line
	}
	return order, lines, nil
}

// inputExpr 计算输入端口的表达式：连线变量 > 覆盖常量 > 默认常量。
func inputExpr(g *graph.Graph, n graph.Node, pd nodelib.PortDef, lib *nodelib.Library, version int) (string, error) {
	if e, ok := findIncoming(g, n.ID, pd.Name); ok {
		return varName(e.From.Node, e.From.Port), nil
	}
	if ov, ok := n.Overrides[pd.Name]; ok && ov != nil {
		conv, ok := graph.Convert(*ov, pd.Type)
		if !ok {
			return "", fmt.Errorf("node %q port %q: cannot convert override %s to %s", n.ID, pd.Name, ov.Type, pd.Type)
		}
		return conv.Literal(), nil
	}
	if pd.Default != nil {
		return pd.Default.Literal(), nil
	}
	return "", fmt.Errorf("node %q port %q: no input available", n.ID, pd.Name)
}

// topoSort 返回确定的拓扑序：Kahn 算法，就绪集按节点 ID 字典序取最小。
func topoSort(g *graph.Graph) ([]graph.NodeID, error) {
	indeg := make(map[graph.NodeID]int, len(g.Nodes))
	adj := make(map[graph.NodeID][]graph.NodeID, len(g.Nodes))
	for _, n := range g.Nodes {
		indeg[n.ID] = 0
	}
	for _, e := range g.Edges {
		adj[e.From.Node] = append(adj[e.From.Node], e.To.Node)
		indeg[e.To.Node]++
	}
	var ready []graph.NodeID
	for id, d := range indeg {
		if d == 0 {
			ready = append(ready, id)
		}
	}
	sort.Slice(ready, func(i, j int) bool { return ready[i] < ready[j] })
	var order []graph.NodeID
	for len(ready) > 0 {
		n := ready[0]
		ready = ready[1:]
		order = append(order, n)
		nexts := adj[n]
		sort.Slice(nexts, func(i, j int) bool { return nexts[i] < nexts[j] })
		for _, m := range nexts {
			indeg[m]--
			if indeg[m] == 0 {
				// 插入并保持 ready 有序。
				i := sort.Search(len(ready), func(i int) bool { return ready[i] >= m })
				ready = append(ready, "")
				copy(ready[i+1:], ready[i:])
				ready[i] = m
			}
		}
	}
	if len(order) != len(g.Nodes) {
		return nil, fmt.Errorf("internal: cycle detected after validation")
	}
	return order, nil
}

// varName 由节点 ID 与端口名确定性地派生变量名。
func varName(n graph.NodeID, p graph.PortID) string {
	return "v_" + sanitize(string(n)) + "_" + sanitize(string(p))
}

func sanitize(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			sb.WriteRune(r)
		} else {
			fmt.Fprintf(&sb, "_x%02x", r)
		}
	}
	return sb.String()
}
