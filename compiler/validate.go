package compiler

import (
	"fmt"
	"sort"

	"github.com/shikejing/material-graph-compiler-go/graph"
	"github.com/shikejing/material-graph-compiler-go/nodelib"
)

// Validate 对图做整体校验：节点类型存在、端口存在、连线类型兼容、无环。
// 任一失败都会返回 *graph.Error（含全部诊断），调用方不得产出任何部分结果。
// 诊断按 (节点, 端口, 诊断码) 排序，保证输出确定。
func Validate(g *graph.Graph, lib *nodelib.Library, version int) *graph.Error {
	var diags []graph.Diagnostic

	// 节点 ID 唯一性 + 节点类型存在性。
	defs := make(map[graph.NodeID]*nodelib.NodeDef, len(g.Nodes))
	seen := make(map[graph.NodeID]bool, len(g.Nodes))
	for _, n := range g.Nodes {
		if seen[n.ID] {
			diags = append(diags, graph.Diagnostic{
				Code:    "DUPLICATE_NODE",
				Message: fmt.Sprintf("duplicate node id %q", n.ID),
				Node:    n.ID,
			})
			continue
		}
		seen[n.ID] = true
		def, ok := lib.Def(version, n.Type)
		if !ok {
			diags = append(diags, graph.Diagnostic{
				Code:    "UNKNOWN_NODE_TYPE",
				Message: fmt.Sprintf("node %q has unknown type %q in library version %d", n.ID, n.Type, version),
				Node:    n.ID,
			})
			continue
		}
		defs[n.ID] = def
		// 覆盖值必须指向存在的输入端口且类型兼容。
		for port, v := range n.Overrides {
			pd, ok := def.Input(port)
			if !ok {
				diags = append(diags, graph.Diagnostic{
					Code:    "PORT_NOT_FOUND",
					Message: fmt.Sprintf("node %q: override targets unknown input port %q", n.ID, port),
					Node:    n.ID, Port: port,
				})
				continue
			}
			if v != nil && !nodelib.Compatible(v.Type, pd.Type) {
				diags = append(diags, graph.Diagnostic{
					Code:    "TYPE_MISMATCH",
					Message: fmt.Sprintf("node %q: override on port %q has type %s, want %s", n.ID, port, v.Type, pd.Type),
					Node:    n.ID, Port: port,
				})
			}
		}
	}

	// 输出节点与端口存在性。
	if _, ok := defs[g.OutputNode]; !ok {
		diags = append(diags, graph.Diagnostic{
			Code:    "PORT_NOT_FOUND",
			Message: fmt.Sprintf("output node %q does not exist or is invalid", g.OutputNode),
			Node:    g.OutputNode,
		})
	} else if _, ok := defs[g.OutputNode].Output(g.OutputPort); !ok {
		diags = append(diags, graph.Diagnostic{
			Code:    "PORT_NOT_FOUND",
			Message: fmt.Sprintf("output node %q has no output port %q", g.OutputNode, g.OutputPort),
			Node:    g.OutputNode, Port: g.OutputPort,
		})
	}

	// 连线校验：两端端口存在、方向正确、类型兼容、输入端口至多一条入线。
	inEdge := make(map[graph.PortRef]bool)
	for _, e := range g.Edges {
		fromDef, okFrom := defs[e.From.Node]
		toDef, okTo := defs[e.To.Node]
		if !okFrom {
			diags = append(diags, graph.Diagnostic{
				Code:    "PORT_NOT_FOUND",
				Message: fmt.Sprintf("edge source node %q does not exist or is invalid", e.From.Node),
				Node:    e.From.Node, Port: e.From.Port,
			})
			continue
		}
		if !okTo {
			diags = append(diags, graph.Diagnostic{
				Code:    "PORT_NOT_FOUND",
				Message: fmt.Sprintf("edge target node %q does not exist or is invalid", e.To.Node),
				Node:    e.To.Node, Port: e.To.Port,
			})
			continue
		}
		outPd, ok := fromDef.Output(e.From.Port)
		if !ok {
			diags = append(diags, graph.Diagnostic{
				Code:    "PORT_NOT_FOUND",
				Message: fmt.Sprintf("node %q has no output port %q", e.From.Node, e.From.Port),
				Node:    e.From.Node, Port: e.From.Port,
			})
			continue
		}
		inPd, ok := toDef.Input(e.To.Port)
		if !ok {
			diags = append(diags, graph.Diagnostic{
				Code:    "PORT_NOT_FOUND",
				Message: fmt.Sprintf("node %q has no input port %q", e.To.Node, e.To.Port),
				Node:    e.To.Node, Port: e.To.Port,
			})
			continue
		}
		if !nodelib.Compatible(outPd.Type, inPd.Type) {
			diags = append(diags, graph.Diagnostic{
				Code:    "TYPE_MISMATCH",
				Message: fmt.Sprintf("edge %s.%s (%s) -> %s.%s (%s): incompatible types",
					e.From.Node, e.From.Port, outPd.Type, e.To.Node, e.To.Port, inPd.Type),
				Node: e.To.Node, Port: e.To.Port,
			})
		}
		if inEdge[e.To] {
			diags = append(diags, graph.Diagnostic{
				Code:    "DUPLICATE_INPUT_EDGE",
				Message: fmt.Sprintf("input port %s.%s has more than one incoming edge", e.To.Node, e.To.Port),
				Node:    e.To.Node, Port: e.To.Port,
			})
		}
		inEdge[e.To] = true
	}

	// 环检测（仅当无结构性错误时进行，避免在残缺图上误报）。
	if len(diags) == 0 {
		diags = append(diags, detectCycles(g)...)
		// 必填输入检查：无默认值、无覆盖、无连线的输入端口。
		inEdge := make(map[graph.PortRef]bool)
		for _, e := range g.Edges {
			inEdge[e.To] = true
		}
		for _, n := range g.Nodes {
			def := defs[n.ID]
			if def == nil {
				continue
			}
			for _, pd := range def.Inputs {
				if pd.Default != nil {
					continue
				}
				if _, ok := n.Overrides[pd.Name]; ok {
					continue
				}
				if inEdge[graph.PortRef{Node: n.ID, Port: pd.Name}] {
					continue
				}
				diags = append(diags, graph.Diagnostic{
					Code:    "MISSING_INPUT",
					Message: fmt.Sprintf("node %q: required input port %q has no edge, override or default", n.ID, pd.Name),
					Node:    n.ID, Port: pd.Name,
				})
			}
		}
	}

	if len(diags) > 0 {
		sort.Slice(diags, func(i, j int) bool {
			a, b := diags[i], diags[j]
			if a.Node != b.Node {
				return a.Node < b.Node
			}
			if a.Port != b.Port {
				return a.Port < b.Port
			}
			return a.Code < b.Code
		})
		return &graph.Error{Diags: diags}
	}
	return nil
}

// detectCycles 用三色 DFS 检测环，返回环上节点的诊断（按节点 ID 排序）。
func detectCycles(g *graph.Graph) []graph.Diagnostic {
	adj := make(map[graph.NodeID][]graph.NodeID, len(g.Nodes))
	for _, e := range g.Edges {
		adj[e.From.Node] = append(adj[e.From.Node], e.To.Node)
	}
	// 邻接表排序，保证遍历顺序确定。
	for id := range adj {
		sort.Slice(adj[id], func(i, j int) bool { return adj[id][i] < adj[id][j] })
	}
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[graph.NodeID]int, len(g.Nodes))
	inCycle := make(map[graph.NodeID]bool)
	var stack []graph.NodeID
	var dfs func(n graph.NodeID)
	dfs = func(n graph.NodeID) {
		color[n] = gray
		stack = append(stack, n)
		for _, m := range adj[n] {
			if color[m] == gray {
				// 找到环：栈中从 m 到栈顶都在环上。
				for i := len(stack) - 1; i >= 0; i-- {
					inCycle[stack[i]] = true
					if stack[i] == m {
						break
					}
				}
			} else if color[m] == white {
				dfs(m)
			}
		}
		stack = stack[:len(stack)-1]
		color[n] = black
	}
	ids := make([]graph.NodeID, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		ids = append(ids, n.ID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		if color[id] == white {
			dfs(id)
		}
	}
	if len(inCycle) == 0 {
		return nil
	}
	cyc := make([]graph.NodeID, 0, len(inCycle))
	for id := range inCycle {
		cyc = append(cyc, id)
	}
	sort.Slice(cyc, func(i, j int) bool { return cyc[i] < cyc[j] })
	diags := make([]graph.Diagnostic, 0, len(cyc))
	for _, id := range cyc {
		diags = append(diags, graph.Diagnostic{
			Code:    "CYCLE",
			Message: fmt.Sprintf("node %q participates in a dependency cycle", id),
			Node:    id,
		})
	}
	return diags
}
