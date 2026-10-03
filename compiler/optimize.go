package compiler

import (
	"sort"

	"github.com/shikejing/material-graph-compiler-go/graph"
	"github.com/shikejing/material-graph-compiler-go/nodelib"
)

// optimize 对图做编译期优化：常量折叠 + 不可达节点消除。
// 对外可观测契约（输出节点/端口及其表达式语义）保持不变；
// 返回优化后的图与被消除节点的诊断（按节点 ID 排序，确定）。
func optimize(g *graph.Graph, lib *nodelib.Library, version int) (*graph.Graph, []Elimination) {
	work := cloneGraph(g)
	var elims []Elimination

	// 常量折叠：迭代到不动点。被折叠节点的输出常量会改写为下游端口的覆盖值。
	constVals := map[graph.PortRef]graph.Value{}
	for {
		foldedAny := false
		// 每轮先取排序后的 ID 快照，循环内再取节点的最新状态，
		// 因为本轮中其他节点折叠可能已改写该节点的覆盖值。
		ids := make([]graph.NodeID, 0, len(work.Nodes))
		for _, n := range work.Nodes {
			ids = append(ids, n.ID)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, id := range ids {
			np := findNode(work, id)
			if np == nil {
				continue // 本轮已被折叠移除
			}
			n := *np
			if n.ID == work.OutputNode {
				continue // 输出节点不可折叠，保持输出契约
			}
			def, ok := lib.Def(version, n.Type)
			if !ok || def.Fold == nil || len(def.Outputs) != 1 {
				continue
			}
			inputs, allConst := constInputs(work, n, def, constVals)
			if !allConst {
				continue
			}
			out, ok := def.Fold(inputs)
			if !ok {
				continue
			}
			outRef := graph.PortRef{Node: n.ID, Port: def.Outputs[0].Name}
			constVals[outRef] = out
			work = removeNode(work, n.ID)
			// 把折叠结果写为下游端口的覆盖常量。
			kept := work.Edges[:0]
			for _, e := range work.Edges {
				if e.From == outRef {
					if tgt := findNode(work, e.To.Node); tgt != nil {
						if tdef, ok := lib.Def(version, tgt.Type); ok {
							if pd, ok := tdef.Input(e.To.Port); ok {
								if conv, ok := graph.Convert(out, pd.Type); ok {
									if tgt.Overrides == nil {
										tgt.Overrides = map[graph.PortID]*graph.Value{}
									}
									c := conv
									tgt.Overrides[e.To.Port] = &c
								}
							}
						}
					}
					continue // 丢弃该连线
				}
				kept = append(kept, e)
			}
			work.Edges = kept
			elims = append(elims, Elimination{Node: n.ID, Reason: "constant-folded: all inputs are compile-time constants"})
			foldedAny = true
		}
		if !foldedAny {
			break
		}
	}

	// 不可达消除：从输出节点沿连线反向可达的节点才保留。
	reachable := map[graph.NodeID]bool{work.OutputNode: true}
	changed := true
	for changed {
		changed = false
		for _, e := range work.Edges {
			if reachable[e.To.Node] && !reachable[e.From.Node] {
				reachable[e.From.Node] = true
				changed = true
			}
		}
	}
	var keptNodes []graph.Node
	for _, n := range work.Nodes {
		if reachable[n.ID] {
			keptNodes = append(keptNodes, n)
		} else {
			elims = append(elims, Elimination{Node: n.ID, Reason: "unreachable: no path to the output node"})
		}
	}
	work.Nodes = keptNodes
	keptEdges := work.Edges[:0]
	for _, e := range work.Edges {
		if reachable[e.From.Node] && reachable[e.To.Node] {
			keptEdges = append(keptEdges, e)
		}
	}
	work.Edges = keptEdges

	sort.Slice(elims, func(i, j int) bool { return elims[i].Node < elims[j].Node })
	return work, elims
}

// constInputs 收集节点的常量输入；任一输入不是编译期常量时 allConst=false。
func constInputs(g *graph.Graph, n graph.Node, def *nodelib.NodeDef, constVals map[graph.PortRef]graph.Value) (map[graph.PortID]graph.Value, bool) {
	inputs := make(map[graph.PortID]graph.Value, len(def.Inputs))
	for _, pd := range def.Inputs {
		if e, ok := findIncoming(g, n.ID, pd.Name); ok {
			cv, ok := constVals[e.From]
			if !ok {
				return nil, false
			}
			conv, ok := graph.Convert(cv, pd.Type)
			if !ok {
				return nil, false
			}
			inputs[pd.Name] = conv
			continue
		}
		if ov, ok := n.Overrides[pd.Name]; ok && ov != nil {
			conv, ok := graph.Convert(*ov, pd.Type)
			if !ok {
				return nil, false
			}
			inputs[pd.Name] = conv
			continue
		}
		if pd.Default != nil {
			inputs[pd.Name] = *pd.Default
			continue
		}
		return nil, false
	}
	return inputs, true
}

func findIncoming(g *graph.Graph, node graph.NodeID, port graph.PortID) (graph.Edge, bool) {
	for _, e := range g.Edges {
		if e.To.Node == node && e.To.Port == port {
			return e, true
		}
	}
	return graph.Edge{}, false
}

func findNode(g *graph.Graph, id graph.NodeID) *graph.Node {
	for i := range g.Nodes {
		if g.Nodes[i].ID == id {
			return &g.Nodes[i]
		}
	}
	return nil
}

func removeNode(g *graph.Graph, id graph.NodeID) *graph.Graph {
	var kept []graph.Node
	for _, n := range g.Nodes {
		if n.ID != id {
			kept = append(kept, n)
		}
	}
	g.Nodes = kept
	return g
}

// cloneGraph 深拷贝图，避免优化过程修改调用方的图。
func cloneGraph(g *graph.Graph) *graph.Graph {
	out := &graph.Graph{
		Nodes:      make([]graph.Node, len(g.Nodes)),
		Edges:      append([]graph.Edge(nil), g.Edges...),
		OutputNode: g.OutputNode,
		OutputPort: g.OutputPort,
	}
	for i, n := range g.Nodes {
		out.Nodes[i] = n
		if n.Overrides != nil {
			ov := make(map[graph.PortID]*graph.Value, len(n.Overrides))
			for p, v := range n.Overrides {
				if v == nil {
					ov[p] = nil
				} else {
					c := *v
					ov[p] = &c
				}
			}
			out.Nodes[i].Overrides = ov
		}
	}
	return out
}
