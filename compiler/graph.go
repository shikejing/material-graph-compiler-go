package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// Node is a single node instance in a material graph.
type Node struct {
	ID     string
	Type   string
	// TypeVersion is the version of the node type this instance was authored
	// against. Migration rules bring it up to the library's current version.
	TypeVersion int
	// Inputs holds explicit constant overrides for input ports. Ports not
	// present here and not connected via edges use the definition default.
	Inputs map[string]Value
	// Props holds non-port string properties (e.g. parameter names).
	Props map[string]string
}

// Edge connects an output port of one node to an input port of another.
type Edge struct {
	FromNode string
	FromPort string
	ToNode   string
	ToPort   string
}

// Graph is a material graph: nodes, edges, and a designated output node.
type Graph struct {
	Nodes      map[string]*Node
	Edges      []*Edge
	OutputNode string
}

// NewGraph creates an empty graph.
func NewGraph() *Graph { return &Graph{Nodes: map[string]*Node{}} }

// AddNode adds a node to the graph.
func (g *Graph) AddNode(n *Node) {
	if g.Nodes == nil {
		g.Nodes = map[string]*Node{}
	}
	g.Nodes[n.ID] = n
}

// AddEdge adds an edge to the graph.
func (g *Graph) AddEdge(e *Edge) {
	g.Edges = append(g.Edges, e)
}

// NodeIDs returns node IDs in deterministic sorted order.
func (g *Graph) NodeIDs() []string {
	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// edgeKey returns a canonical string for an edge.
func edgeKey(e *Edge) string {
	return e.FromNode + "." + e.FromPort + "->" + e.ToNode + "." + e.ToPort
}

// Canonical returns a deterministic string representation for hashing. Nodes
// and edges are sorted so that traversal/insertion order cannot affect the
// result.
func (g *Graph) Canonical() string {
	var b strings.Builder
	b.WriteString("output=")
	b.WriteString(g.OutputNode)
	b.WriteByte('\n')
	for _, id := range g.NodeIDs() {
		n := g.Nodes[id]
		b.WriteString("node=")
		b.WriteString(n.ID)
		b.WriteString(" type=")
		b.WriteString(n.Type)
		b.WriteString(" v=")
		b.WriteString(itoa(n.TypeVersion))
		// sorted input ports
		ports := make([]string, 0, len(n.Inputs))
		for p := range n.Inputs {
			ports = append(ports, p)
		}
		sort.Strings(ports)
		for _, p := range ports {
			b.WriteByte(' ')
			b.WriteString(p)
			b.WriteByte('=')
			b.WriteString(n.Inputs[p].Canonical())
		}
		// sorted props
		props := make([]string, 0, len(n.Props))
		for p := range n.Props {
			props = append(props, p)
		}
		sort.Strings(props)
		for _, p := range props {
			b.WriteByte(' ')
			b.WriteString(p)
			b.WriteByte('"')
			b.WriteString(n.Props[p])
			b.WriteByte('"')
		}
		b.WriteByte('\n')
	}
	// sorted edges
	edges := make([]*Edge, len(g.Edges))
	copy(edges, g.Edges)
	sort.Slice(edges, func(i, j int) bool { return edgeKey(edges[i]) < edgeKey(edges[j]) })
	for _, e := range edges {
		b.WriteString("edge=")
		b.WriteString(edgeKey(e))
		b.WriteByte('\n')
	}
	return b.String()
}

// Hash returns a deterministic content hash of the graph.
func (g *Graph) Hash() string {
	sum := sha256.Sum256([]byte(g.Canonical()))
	return hex.EncodeToString(sum[:])
}

// itoa is a small int-to-string helper to avoid importing strconv in hot paths.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
