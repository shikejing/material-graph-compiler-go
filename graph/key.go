package graph

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
)

// Key 返回图内容的稳定指纹：只由节点、连线、覆盖值与输出声明决定，
// 与切片顺序、遍历顺序无关。用于缓存键与增量比较。
func (g *Graph) Key() string {
	h := fnv.New64a()

	nodes := make([]Node, len(g.Nodes))
	copy(nodes, g.Nodes)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	for _, n := range nodes {
		fmt.Fprintf(h, "node|%s|%s", n.ID, n.Type)
		ports := make([]string, 0, len(n.Overrides))
		vals := make(map[string]string, len(n.Overrides))
		for p, v := range n.Overrides {
			ports = append(ports, string(p))
			if v == nil {
				vals[string(p)] = "nil"
			} else {
				vals[string(p)] = v.Canonical()
			}
		}
		sort.Strings(ports)
		for _, p := range ports {
			fmt.Fprintf(h, "|ovr|%s=%s", p, vals[p])
		}
		fmt.Fprintf(h, "\n")
	}

	edges := make([]string, len(g.Edges))
	for i, e := range g.Edges {
		edges[i] = fmt.Sprintf("%s.%s>%s.%s", e.From.Node, e.From.Port, e.To.Node, e.To.Port)
	}
	sort.Strings(edges)
	for _, e := range edges {
		fmt.Fprintf(h, "edge|%s\n", e)
	}

	fmt.Fprintf(h, "out|%s.%s\n", g.OutputNode, g.OutputPort)
	return fmt.Sprintf("%x", h.Sum64())
}

// NodeKey 返回单个节点内容的稳定指纹（类型 + 覆盖值），用于增量重编。
func NodeKey(n Node) string {
	h := fnv.New64a()
	fmt.Fprintf(h, "node|%s|%s", n.ID, n.Type)
	ports := make([]string, 0, len(n.Overrides))
	for p := range n.Overrides {
		ports = append(ports, string(p))
	}
	sort.Strings(ports)
	var sb strings.Builder
	for _, p := range ports {
		v := n.Overrides[PortID(p)]
		s := "nil"
		if v != nil {
			s = v.Canonical()
		}
		sb.WriteString("|")
		sb.WriteString(p)
		sb.WriteString("=")
		sb.WriteString(s)
	}
	fmt.Fprintf(h, "%s", sb.String())
	return fmt.Sprintf("%x", h.Sum64())
}
