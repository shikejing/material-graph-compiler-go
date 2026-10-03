package compiler

import "sort"

// OptimizedGraph is the result of optimization: a subset of live nodes with
// folded constant values, plus diagnostics explaining eliminations and folds.
type OptimizedGraph struct {
	LiveNodes map[string]bool
	Folded    map[string][]Value // node ID -> folded output values (nil if not folded)
	Diags     []Diagnostic
}

// topologicalOrder returns node IDs in a deterministic topological order
// (dependencies first). Ties are broken by sorted node ID, so the result is
// independent of insertion or traversal order. The graph must be a DAG.
func topologicalOrder(g *Graph) []string {
	indeg := map[string]int{}
	for _, id := range g.NodeIDs() {
		indeg[id] = 0
	}
	for _, e := range g.Edges {
		indeg[e.ToNode]++
	}
	ready := make([]string, 0)
	for id, d := range indeg {
		if d == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)
	var order []string
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		order = append(order, id)
		// Collect newly ready dependents, then re-sort for determinism.
		var newly []string
		for _, e := range g.Edges {
			if e.FromNode == id {
				indeg[e.ToNode]--
				if indeg[e.ToNode] == 0 {
					newly = append(newly, e.ToNode)
				}
			}
		}
		sort.Strings(newly)
		ready = append(ready, newly...)
	}
	return order
}

// liveSet returns the set of nodes reachable backward from the output node.
func liveSet(g *Graph) map[string]bool {
	live := map[string]bool{}
	if g.OutputNode == "" {
		return live
	}
	queue := []string{g.OutputNode}
	live[g.OutputNode] = true
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, e := range g.Edges {
			if e.ToNode == id && !live[e.FromNode] {
				live[e.FromNode] = true
				queue = append(queue, e.FromNode)
			}
		}
	}
	return live
}

// Optimize performs constant folding and dead-code elimination. It returns the
// optimized graph and diagnostics. Semantics are preserved: folded nodes are
// replaced by their literal values, and eliminated nodes are unreachable from
// the output.
func Optimize(lib *NodeLibrary, g *Graph) *OptimizedGraph {
	live := liveSet(g)
	folded := map[string][]Value{}
	var diags []Diagnostic

	// Build a lookup of incoming edges per node.
	incoming := map[string][]*Edge{}
	for _, e := range g.Edges {
		incoming[e.ToNode] = append(incoming[e.ToNode], e)
	}

	// Fold live nodes in topological order.
	order := topologicalOrder(g)
	for _, id := range order {
		if !live[id] {
			continue
		}
		n := g.Nodes[id]
		def, ok := lib.Get(n.Type)
		if !ok || def.Eval == nil {
			continue
		}
		// Collect input values in port order.
		inputs := make([]Value, 0, len(def.Inputs))
		allConst := true
		for _, pd := range def.Inputs {
			var val Value
			isConst := false
			// Check connected input.
			var conn *Edge
			for _, e := range incoming[id] {
				if e.ToPort == pd.Name {
					conn = e
					break
				}
			}
			if conn != nil {
				// Connected: the upstream node must be folded.
				upVals, ok := folded[conn.FromNode]
				if !ok {
					allConst = false
					break
				}
				upDef, ok := lib.Get(g.Nodes[conn.FromNode].Type)
				if !ok {
					allConst = false
					break
				}
				outIdx := -1
				for i, op := range upDef.Outputs {
					if op.Name == conn.FromPort {
						outIdx = i
						break
					}
				}
				if outIdx < 0 || outIdx >= len(upVals) {
					allConst = false
					break
				}
				val = upVals[outIdx]
				isConst = true
			} else if ov, ok := n.Inputs[pd.Name]; ok {
				// Literal override.
				val = ov
				isConst = true
			} else {
				// Default.
				val = pd.Default
				isConst = true
			}
			if !isConst {
				allConst = false
				break
			}
			// Coerce to port type.
			if coerced, ok := val.Coerce(pd.Type); ok {
				val = coerced
			}
			inputs = append(inputs, val)
		}
		if !allConst {
			continue
		}
		out := def.Eval(inputs)
		if out == nil {
			continue
		}
		folded[id] = out
		diags = append(diags, Diagnostic{
			Severity: SeverityInfo, Kind: DiagFolded, NodeID: id,
			Message: "constant-folded node " + id,
		})
	}

	// Dead-code elimination diagnostics.
	for _, id := range g.NodeIDs() {
		if !live[id] {
			diags = append(diags, Diagnostic{
				Severity: SeverityInfo, Kind: DiagEliminated, NodeID: id,
				Message: "eliminated unreachable node " + id,
			})
		}
	}

	return &OptimizedGraph{LiveNodes: live, Folded: folded, Diags: diags}
}
