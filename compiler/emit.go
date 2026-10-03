package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// Fragment is the compiled result of a single node: its emitted GLSL statements
// and the variable names of its output ports.
type Fragment struct {
	NodeID   string
	NodeType string
	Version  int
	// Vars maps output port name to GLSL variable name (or literal for folded
	// nodes).
	Vars map[string]string
	// Code is the emitted GLSL statements (empty for folded nodes).
	Code string
	// Folded is true if the node was constant-folded.
	Folded bool
	// Values holds folded output values when Folded is true.
	Values []Value
}

// sanitize converts a string into a valid GLSL identifier component.
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// varName returns the GLSL variable name for a node's output port. Names are
// derived from node ID and port name so they are stable across recompiles and
// independent of traversal order.
func varName(nodeID, port string) string {
	return "v_" + sanitize(nodeID) + "_" + sanitize(port)
}

// FragmentKey computes a content-addressed key for a node fragment. The key
// depends on the node type, version, and the content of its inputs (constant
// values or upstream fragment keys). It is independent of traversal order.
func FragmentKey(lib *NodeLibrary, g *Graph, n *Node, opt *OptimizedGraph, upstreamKeys map[string]string) string {
	var b strings.Builder
	// The fragment is specific to the node ID because variable names are
	// derived from it. This makes the fragment cache safe for incremental
	// recompilation (stable IDs) while preventing incorrect cross-graph reuse.
	b.WriteString(n.ID)
	b.WriteByte(':')
	b.WriteString(n.Type)
	b.WriteByte(':')
	b.WriteString(itoa(n.TypeVersion))
	def, ok := lib.Get(n.Type)
	if !ok {
		b.WriteString(":unknown")
	} else {
		// Build incoming edge lookup.
		incoming := map[string]*Edge{}
		for _, e := range g.Edges {
			if e.ToNode == n.ID {
				incoming[e.ToPort] = e
			}
		}
		ports := make([]string, 0, len(def.Inputs))
		for _, pd := range def.Inputs {
			ports = append(ports, pd.Name)
		}
		sort.Strings(ports)
		for _, p := range ports {
			pd, _ := def.InputDef(p)
			b.WriteByte(' ')
			b.WriteString(p)
			b.WriteByte('=')
			if e, ok := incoming[p]; ok {
				if opt != nil && opt.Folded[e.FromNode] != nil {
					// Upstream is folded; bind to the folded value.
					upDef, _ := lib.Get(g.Nodes[e.FromNode].Type)
					outIdx := -1
					for i, op := range upDef.Outputs {
						if op.Name == e.FromPort {
							outIdx = i
							break
						}
					}
					if outIdx >= 0 && outIdx < len(opt.Folded[e.FromNode]) {
						v := opt.Folded[e.FromNode][outIdx]
						if coerced, ok := v.Coerce(pd.Type); ok {
							v = coerced
						}
						b.WriteString("const:")
						b.WriteString(v.Canonical())
						continue
					}
				}
				if uk, ok := upstreamKeys[e.FromNode]; ok {
					b.WriteString("frag:")
					b.WriteString(uk)
				} else {
					b.WriteString("frag:?")
				}
			} else if ov, ok := n.Inputs[p]; ok {
				v := ov
				if coerced, ok := v.Coerce(pd.Type); ok {
					v = coerced
				}
				b.WriteString("const:")
				b.WriteString(v.Canonical())
			} else {
				v := pd.Default
				if coerced, ok := v.Coerce(pd.Type); ok {
					v = coerced
				}
				b.WriteString("const:")
				b.WriteString(v.Canonical())
			}
		}
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// compileNode emits a single node, returning its fragment. Folded nodes produce
// literal variable mappings and no code.
func compileNode(lib *NodeLibrary, g *Graph, n *Node, opt *OptimizedGraph, upstream map[string]*Fragment) (*Fragment, []Diagnostic) {
	def, ok := lib.Get(n.Type)
	if !ok {
		return nil, []Diagnostic{{
			Severity: SeverityError, Kind: DiagUnknownNodeType, NodeID: n.ID,
			Message: "unknown node type " + n.Type,
		}}
	}
	frag := &Fragment{NodeID: n.ID, NodeType: n.Type, Version: n.TypeVersion, Vars: map[string]string{}}

	// Folded node: map outputs to literals.
	if opt != nil && opt.Folded[n.ID] != nil {
		frag.Folded = true
		frag.Values = opt.Folded[n.ID]
		for i, op := range def.Outputs {
			if i < len(frag.Values) {
				frag.Vars[op.Name] = frag.Values[i].GLSL()
			}
		}
		return frag, nil
	}

	// Build the emit context.
	ctx := &EmitContext{
		node:     n,
		def:      def,
		values:   map[string]Value{},
		exprs:    map[string]string{},
		srcTypes: map[string]PortType{},
	}
	incoming := map[string]*Edge{}
	for _, e := range g.Edges {
		if e.ToNode == n.ID {
			incoming[e.ToPort] = e
		}
	}
	for _, pd := range def.Inputs {
		if e, ok := incoming[pd.Name]; ok {
			upFrag := upstream[e.FromNode]
			if upFrag == nil {
				return nil, []Diagnostic{{
					Severity: SeverityError, Kind: DiagPortNotFound, NodeID: n.ID, Port: pd.Name,
					Message: "upstream fragment not found for " + e.FromNode,
				}}
			}
			upDef, _ := lib.Get(g.Nodes[e.FromNode].Type)
			opd, _ := upDef.OutputDef(e.FromPort)
			ctx.srcTypes[pd.Name] = opd.Type
			if upFrag.Folded {
				// Upstream folded: bind to the literal value.
				idx := outputIndex(upDef, e.FromPort)
				if idx >= 0 && idx < len(upFrag.Values) {
					v := upFrag.Values[idx]
					if coerced, ok := v.Coerce(pd.Type); ok {
						ctx.values[pd.Name] = coerced
					} else {
						ctx.values[pd.Name] = v
					}
				}
			} else {
				ctx.exprs[pd.Name] = upFrag.Vars[e.FromPort]
			}
		} else if ov, ok := n.Inputs[pd.Name]; ok {
			v := ov
			if coerced, ok := v.Coerce(pd.Type); ok {
				v = coerced
			}
			ctx.values[pd.Name] = v
		} else {
			v := pd.Default
			if coerced, ok := v.Coerce(pd.Type); ok {
				v = coerced
			}
			ctx.values[pd.Name] = v
		}
	}

	outs := def.Emit(ctx)
	if outs == nil {
		outs = map[string]string{}
	}
	// Assign variables deterministically in output-port order.
	var b strings.Builder
	for _, op := range def.Outputs {
		expr, ok := outs[op.Name]
		if !ok {
			expr = "0"
		}
		vn := varName(n.ID, op.Name)
		frag.Vars[op.Name] = vn
		b.WriteString("    ")
		b.WriteString(string(op.Type))
		b.WriteByte(' ')
		b.WriteString(vn)
		b.WriteString(" = ")
		b.WriteString(expr)
		b.WriteString(";\n")
	}
	frag.Code = b.String()
	return frag, nil
}

// outputIndex returns the index of an output port in a node definition.
func outputIndex(def *NodeDef, port string) int {
	for i, op := range def.Outputs {
		if op.Name == port {
			return i
		}
	}
	return -1
}
