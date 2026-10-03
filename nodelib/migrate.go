package nodelib

import (
	"fmt"

	"github.com/shikejing/material-graph-compiler-go/graph"
)

// MigrateError 表示旧图无法自动迁移到新库版本。
type MigrateError struct {
	Reasons []string
}

func (e *MigrateError) Error() string {
	if len(e.Reasons) == 0 {
		return "graph migration failed"
	}
	return "graph migration failed: " + e.Reasons[0]
}

// MigrateGraph 把针对 fromVersion 编写的图迁移到 toVersion（仅支持升级）。
// 逐版本应用声明的迁移规则：端口改名、端口删除（连同其连线与覆盖值）、
// 新增端口的默认值补全。无法自动兼容时返回 *MigrateError，原图不被修改，
// 因此已产出的编译结果（基于旧版本的缓存）不受影响。
func MigrateGraph(g *graph.Graph, lib *Library, fromVersion, toVersion int) (*graph.Graph, error) {
	if fromVersion > toVersion {
		return nil, &MigrateError{Reasons: []string{fmt.Sprintf("downgrade from v%d to v%d is not supported", fromVersion, toVersion)}}
	}
	cur := cloneGraph(g)
	for v := fromVersion + 1; v <= toVersion; v++ {
		ver, ok := lib.versions[v]
		if !ok {
			return nil, &MigrateError{Reasons: []string{fmt.Sprintf("library version %d not registered", v)}}
		}
		if err := migrateStep(cur, ver); err != nil {
			return nil, err
		}
	}
	return cur, nil
}

// migrateStep 把图从 ver.Number-1 迁移到 ver.Number。
func migrateStep(g *graph.Graph, ver *Version) error {
	migByType := make(map[string]Migration, len(ver.Migrations))
	for _, m := range ver.Migrations {
		migByType[m.Type] = m
	}
	typeOf := make(map[graph.NodeID]string, len(g.Nodes))
	for _, n := range g.Nodes {
		typeOf[n.ID] = n.Type
	}
	var errs []string

	// 1. 迁移节点覆盖值。
	for i := range g.Nodes {
		n := &g.Nodes[i]
		newDef, ok := ver.Defs[n.Type]
		if !ok {
			errs = append(errs, fmt.Sprintf("node %q: type %q no longer exists in version %d", n.ID, n.Type, ver.Number))
			continue
		}
		if m, ok := migByType[n.Type]; ok {
			newOverrides := make(map[graph.PortID]*graph.Value, len(n.Overrides))
			for p, val := range n.Overrides {
				if np, ok := m.RenamedPorts[p]; ok {
					p = np
				} else if _, ok := m.RemovedPorts[p]; ok {
					continue // 端口被删除，覆盖值一并丢弃
				}
				newOverrides[p] = val
			}
			n.Overrides = newOverrides
			// 新增端口用声明的默认值补全，保证行为确定。
			for p, val := range m.NewDefaults {
				if _, ok := n.Overrides[p]; !ok {
					c := val
					n.Overrides[p] = &c
				}
			}
		}
		// 覆盖值必须指向新定义中存在的输入端口。
		for p := range n.Overrides {
			if _, ok := newDef.Input(p); !ok {
				errs = append(errs, fmt.Sprintf("node %q: override on port %q has no counterpart in version %d and no migration rule", n.ID, p, ver.Number))
			}
		}
	}

	// 2. 迁移连线。
	kept := g.Edges[:0]
	for _, e := range g.Edges {
		drop := false
		if m, ok := migByType[typeOf[e.From.Node]]; ok {
			if np, ok := m.RenamedPorts[e.From.Port]; ok {
				e.From.Port = np
			} else if _, ok := m.RemovedPorts[e.From.Port]; ok {
				drop = true
			}
		}
		if m, ok := migByType[typeOf[e.To.Node]]; ok {
			if np, ok := m.RenamedPorts[e.To.Port]; ok {
				e.To.Port = np
			} else if _, ok := m.RemovedPorts[e.To.Port]; ok {
				drop = true
			}
		}
		if drop {
			continue
		}
		// 连线两端端口必须在新版本中存在。
		if def, ok := ver.Defs[typeOf[e.From.Node]]; ok {
			if _, ok := def.Output(e.From.Port); !ok {
				errs = append(errs, fmt.Sprintf("edge from %s.%s: port missing in version %d and no migration rule", e.From.Node, e.From.Port, ver.Number))
				continue
			}
		}
		if def, ok := ver.Defs[typeOf[e.To.Node]]; ok {
			if _, ok := def.Input(e.To.Port); !ok {
				errs = append(errs, fmt.Sprintf("edge to %s.%s: port missing in version %d and no migration rule", e.To.Node, e.To.Port, ver.Number))
				continue
			}
		}
		kept = append(kept, e)
	}
	g.Edges = kept

	// 3. 迁移输出声明。
	if m, ok := migByType[typeOf[g.OutputNode]]; ok {
		if np, ok := m.RenamedPorts[g.OutputPort]; ok {
			g.OutputPort = np
		} else if _, ok := m.RemovedPorts[g.OutputPort]; ok {
			errs = append(errs, fmt.Sprintf("output port %s.%s removed in version %d", g.OutputNode, g.OutputPort, ver.Number))
		}
	}

	// 4. 新增必填输入（无默认值、无迁移默认值、无连线）无法自动兼容。
	inEdge := make(map[graph.PortRef]bool)
	for _, e := range g.Edges {
		inEdge[e.To] = true
	}
	for _, n := range g.Nodes {
		def, ok := ver.Defs[n.Type]
		if !ok {
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
			errs = append(errs, fmt.Sprintf("node %q: new required input %q in version %d has no default and no migration rule", n.ID, pd.Name, ver.Number))
		}
	}

	if len(errs) > 0 {
		return &MigrateError{Reasons: errs}
	}
	return nil
}

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
