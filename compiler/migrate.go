package compiler

import "sort"

// Migration declares how to upgrade a node instance between two versions of a
// node type. Migrations must be deterministic and total: anything that cannot
// be migrated is reported as an error and the graph is rejected.
type Migration struct {
	NodeType    string
	FromVersion int
	ToVersion   int
	// Renames maps old input port names to new input port names.
	Renames map[string]string
	// OutputRenames maps old output port names to new output port names.
	OutputRenames map[string]string
	// Removed lists input ports that no longer exist. If a graph instance
	// overrides a removed port (non-default value or incoming edge), migration
	// fails.
	Removed []string
	// Added lists new input ports with their defaults.
	Added map[string]Value
	// Defaults lists input ports whose default value changed (new default).
	Defaults map[string]Value
}

// cloneGraph returns a deep copy of the graph.
func cloneGraph(g *Graph) *Graph {
	out := NewGraph()
	out.OutputNode = g.OutputNode
	for id, n := range g.Nodes {
		cn := &Node{ID: n.ID, Type: n.Type, TypeVersion: n.TypeVersion}
		if n.Inputs != nil {
			cn.Inputs = make(map[string]Value, len(n.Inputs))
			for k, v := range n.Inputs {
				cn.Inputs[k] = v
			}
		}
		if n.Props != nil {
			cn.Props = make(map[string]string, len(n.Props))
			for k, v := range n.Props {
				cn.Props[k] = v
			}
		}
		out.Nodes[id] = cn
	}
	for _, e := range g.Edges {
		out.Edges = append(out.Edges, &Edge{
			FromNode: e.FromNode, FromPort: e.FromPort,
			ToNode: e.ToNode, ToPort: e.ToPort,
		})
	}
	return out
}

// MigrateGraph upgrades every node in the graph to the library's current node
// versions, returning a new graph and any diagnostics. The original graph is
// not modified.
func MigrateGraph(lib *NodeLibrary, g *Graph) (*Graph, []Diagnostic) {
	out := cloneGraph(g)
	var diags []Diagnostic

	// Build a set of connected input ports (toNode.toPort) for the removed-port
	// check.
	connected := map[string]bool{}
	for _, e := range g.Edges {
		connected[e.ToNode+"."+e.ToPort] = true
	}

	// Sort node IDs for deterministic diagnostics.
	ids := out.NodeIDs()
	for _, id := range ids {
		n := out.Nodes[id]
		def, ok := lib.Get(n.Type)
		if !ok {
			diags = append(diags, Diagnostic{
				Severity: SeverityError, Kind: DiagUnknownNodeType, NodeID: id,
				Message: "unknown node type " + n.Type,
			})
			continue
		}
		if n.TypeVersion > def.Version {
			diags = append(diags, Diagnostic{
				Severity: SeverityError, Kind: DiagMigrationFailed, NodeID: id,
				Message: "node type version " + itoa(n.TypeVersion) + " is newer than library version " + itoa(def.Version),
			})
			continue
		}
		// TypeVersion 0 means "authored against the current library version";
		// no migration is needed.
		if n.TypeVersion == 0 {
			n.TypeVersion = def.Version
		}
		if n.TypeVersion == def.Version {
			continue
		}
		migs := lib.Migrations(n.Type)
		current := n.TypeVersion
		for current < def.Version {
			var m *Migration
			for i := range migs {
				if migs[i].FromVersion == current {
					m = &migs[i]
					break
				}
			}
			if m == nil {
				diags = append(diags, Diagnostic{
					Severity: SeverityError, Kind: DiagMigrationFailed, NodeID: id,
					Message: "no migration path from version " + itoa(current) + " to " + itoa(def.Version),
				})
				break
			}
			if m.ToVersion <= m.FromVersion {
				diags = append(diags, Diagnostic{
					Severity: SeverityError, Kind: DiagMigrationFailed, NodeID: id,
					Message: "invalid migration " + itoa(m.FromVersion) + "->" + itoa(m.ToVersion),
				})
				break
			}
			// Apply renames to input overrides.
			for oldName, newName := range m.Renames {
				if v, ok := n.Inputs[oldName]; ok {
					delete(n.Inputs, oldName)
					n.Inputs[newName] = v
				}
			}
			// Apply renames to edges (to ports).
			for _, e := range out.Edges {
				if e.ToNode == id {
					if newName, ok := m.Renames[e.ToPort]; ok {
						e.ToPort = newName
					}
				}
			}
			// Apply output renames to edges (from ports).
			for _, e := range out.Edges {
				if e.FromNode == id {
					if newName, ok := m.OutputRenames[e.FromPort]; ok {
						e.FromPort = newName
					}
				}
			}
			// Handle removed ports: reject if overridden or connected.
			for _, port := range m.Removed {
				if _, overridden := n.Inputs[port]; overridden {
					diags = append(diags, Diagnostic{
						Severity: SeverityError, Kind: DiagMigrationFailed, NodeID: id, Port: port,
						Message: "removed input port " + port + " has an explicit value; cannot migrate",
					})
					continue
				}
				if connected[id+"."+port] {
					diags = append(diags, Diagnostic{
						Severity: SeverityError, Kind: DiagMigrationFailed, NodeID: id, Port: port,
						Message: "removed input port " + port + " is connected; cannot migrate",
					})
					continue
				}
				delete(n.Inputs, port)
			}
			// Added ports: defaults come from the current definition; no
			// action needed. Defaults changes likewise apply automatically to
			// non-overridden ports.
			current = m.ToVersion
		}
		n.TypeVersion = def.Version
	}
	// Re-sort edges deterministically after renames.
	sort.Slice(out.Edges, func(i, j int) bool { return edgeKey(out.Edges[i]) < edgeKey(out.Edges[j]) })
	return out, diags
}
