package compiler

// ValidateGraph checks the graph against the library: node types exist, ports
// exist, types are compatible, each input is connected at most once, the
// output node is valid, and there are no cycles. It returns all diagnostics;
// the graph is accepted only if no error diagnostic is present.
func ValidateGraph(lib *NodeLibrary, g *Graph) []Diagnostic {
	var diags []Diagnostic

	// 1. Output node designation.
	if g.OutputNode == "" {
		diags = append(diags, Diagnostic{
			Severity: SeverityError, Kind: DiagNoOutput,
			Message: "graph has no designated output node",
		})
	} else {
		out, ok := g.Nodes[g.OutputNode]
		if !ok {
			diags = append(diags, Diagnostic{
				Severity: SeverityError, Kind: DiagOutputNotFound, NodeID: g.OutputNode,
				Message: "designated output node not found",
			})
		} else if out.Type != OutputNodeType {
			diags = append(diags, Diagnostic{
				Severity: SeverityError, Kind: DiagNoOutput, NodeID: g.OutputNode,
				Message: "designated output node is of type " + out.Type + ", want " + OutputNodeType,
			})
		}
	}
	// Count output nodes; require exactly one.
	outCount := 0
	for _, n := range g.Nodes {
		if n.Type == OutputNodeType {
			outCount++
		}
	}
	if outCount > 1 {
		diags = append(diags, Diagnostic{
			Severity: SeverityError, Kind: DiagMultipleOutputs,
			Message: "graph has multiple output nodes; exactly one is required",
		})
	}

	// 2. Node types and input values.
	for _, id := range g.NodeIDs() {
		n := g.Nodes[id]
		def, ok := lib.Get(n.Type)
		if !ok {
			diags = append(diags, Diagnostic{
				Severity: SeverityError, Kind: DiagUnknownNodeType, NodeID: id,
				Message: "unknown node type " + n.Type,
			})
			continue
		}
		for port, val := range n.Inputs {
			pd, ok := def.InputDef(port)
			if !ok {
				diags = append(diags, Diagnostic{
					Severity: SeverityError, Kind: DiagPortNotFound, NodeID: id, Port: port,
					Message: "node " + n.Type + " has no input port " + port,
				})
				continue
			}
			if !Compatible(val.Type, pd.Type) {
				diags = append(diags, Diagnostic{
					Severity: SeverityError, Kind: DiagTypeMismatch, NodeID: id, Port: port,
					Message: "input port " + port + " of type " + string(pd.Type) + " got value of type " + string(val.Type),
				})
			}
		}
	}

	// 3. Edges: existence, single-connection, type compatibility.
	connected := map[string]bool{}
	for _, e := range g.Edges {
		fromNode, ok := g.Nodes[e.FromNode]
		if !ok {
			diags = append(diags, Diagnostic{
				Severity: SeverityError, Kind: DiagPortNotFound, NodeID: e.FromNode, Port: e.FromPort,
				Message: "edge source node not found",
			})
			continue
		}
		toNode, ok := g.Nodes[e.ToNode]
		if !ok {
			diags = append(diags, Diagnostic{
				Severity: SeverityError, Kind: DiagPortNotFound, NodeID: e.ToNode, Port: e.ToPort,
				Message: "edge target node not found",
			})
			continue
		}
		fromDef, ok := lib.Get(fromNode.Type)
		if !ok {
			continue // already reported
		}
		toDef, ok := lib.Get(toNode.Type)
		if !ok {
			continue
		}
		fromPort, ok := fromDef.OutputDef(e.FromPort)
		if !ok {
			diags = append(diags, Diagnostic{
				Severity: SeverityError, Kind: DiagPortNotFound, NodeID: e.FromNode, Port: e.FromPort,
				Message: "node " + fromNode.Type + " has no output port " + e.FromPort,
			})
		}
		toPort, ok := toDef.InputDef(e.ToPort)
		if !ok {
			diags = append(diags, Diagnostic{
				Severity: SeverityError, Kind: DiagPortNotFound, NodeID: e.ToNode, Port: e.ToPort,
				Message: "node " + toNode.Type + " has no input port " + e.ToPort,
			})
			continue
		}
		key := e.ToNode + "." + e.ToPort
		if connected[key] {
			diags = append(diags, Diagnostic{
				Severity: SeverityError, Kind: DiagTypeMismatch, NodeID: e.ToNode, Port: e.ToPort,
				Message: "input port " + e.ToPort + " is connected more than once",
			})
		}
		connected[key] = true
		if fromPort.Type != "" && toPort.Type != "" && !Compatible(fromPort.Type, toPort.Type) {
			diags = append(diags, Diagnostic{
				Severity: SeverityError, Kind: DiagTypeMismatch, NodeID: e.ToNode, Port: e.ToPort,
				Message: "edge from " + string(fromPort.Type) + " port " + e.FromNode + "." + e.FromPort +
					" to " + string(toPort.Type) + " port " + e.ToNode + "." + e.ToPort + " is incompatible",
			})
		}
	}

	// 4. Cycle detection (DFS with coloring).
	if cyc := detectCycle(g); cyc != nil {
		diags = append(diags, cyc...)
	}
	return diags
}

// detectCycle returns diagnostics for every cycle found, locating the nodes
// and ports involved.
func detectCycle(g *Graph) []Diagnostic {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	var diags []Diagnostic
	var stack []string

	// Build adjacency: node -> list of (port, targetNode, targetPort).
	type adj struct {
		fromPort string
		toNode   string
		toPort   string
	}
	adjMap := map[string][]adj{}
	for _, e := range g.Edges {
		adjMap[e.FromNode] = append(adjMap[e.FromNode], adj{e.FromPort, e.ToNode, e.ToPort})
	}

	var dfs func(id string)
	dfs = func(id string) {
		color[id] = gray
		stack = append(stack, id)
		for _, a := range adjMap[id] {
			c := color[a.toNode]
			if c == gray {
				// Found a cycle: report the path from a.toNode to id.
				diags = append(diags, Diagnostic{
					Severity: SeverityError, Kind: DiagCycle, NodeID: id, Port: a.fromPort,
					Message: "cycle detected involving " + a.toNode + " -> " + id,
				})
			} else if c == white {
				dfs(a.toNode)
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = black
	}

	for _, id := range g.NodeIDs() {
		if color[id] == white {
			dfs(id)
		}
	}
	return diags
}
