package compiler

import "sort"

// OutputNodeType is the built-in material output node type.
const OutputNodeType = "output"

// PortDef describes an input or output port of a node type.
type PortDef struct {
	Name    string
	Type    PortType
	Default Value
}

// EmitContext provides input expressions to a node's Emit function.
type EmitContext struct {
	node     *Node
	def      *NodeDef
	values   map[string]Value    // constant values per input port (literals, defaults, folded)
	exprs    map[string]string   // GLSL expressions per input port (connected)
	srcTypes map[string]PortType // source type per input port (for coercion casts)
}

// InputExpr returns the GLSL expression for the named input port. Constant
// inputs (literals, defaults, or folded values) are emitted as literals;
// connected inputs reference the upstream variable, wrapped in a coercion
// cast when the source type differs from the port type.
func (c *EmitContext) InputExpr(name string) string {
	if v, ok := c.values[name]; ok {
		return v.GLSL()
	}
	expr := c.exprs[name]
	pd, ok := c.def.InputDef(name)
	if !ok {
		return expr
	}
	src := c.srcTypes[name]
	if src == "" || src == pd.Type {
		return expr
	}
	if !Compatible(src, pd.Type) {
		return expr
	}
	// Insert a coercion cast.
	switch pd.Type {
	case TypeFloat:
		return "float(" + expr + ")"
	case TypeVec2:
		return "vec2(" + expr + ")"
	case TypeVec3:
		if src == TypeColor {
			return expr // color is vec3
		}
		return "vec3(" + expr + ")"
	case TypeVec4:
		return "vec4(" + expr + ")"
	case TypeColor:
		return expr // color is vec3
	}
	return expr
}

// InputValue returns the constant value of the named input port and whether it
// is a constant (literal, default, or folded).
func (c *EmitContext) InputValue(name string) (Value, bool) {
	v, ok := c.values[name]
	return v, ok
}

// NodeID returns the current node's ID.
func (c *EmitContext) NodeID() string { return c.node.ID }

// NodeDef describes a node type in the node library.
type NodeDef struct {
	Type    string
	Version int
	Inputs  []PortDef
	Outputs []PortDef
	// Emit generates GLSL expressions for each output port.
	Emit func(c *EmitContext) map[string]string
	// Eval optionally evaluates constant inputs to produce constant outputs,
	// enabling constant folding. The argument is the input values in port
	// order. Returns nil if the node cannot be folded.
	Eval func(inputs []Value) []Value
}

// InputDef returns the input port definition with the given name.
func (d *NodeDef) InputDef(name string) (PortDef, bool) {
	for _, p := range d.Inputs {
		if p.Name == name {
			return p, true
		}
	}
	return PortDef{}, false
}

// OutputDef returns the output port definition with the given name.
func (d *NodeDef) OutputDef(name string) (PortDef, bool) {
	for _, p := range d.Outputs {
		if p.Name == name {
			return p, true
		}
	}
	return PortDef{}, false
}

// NodeLibrary is a versioned registry of node type definitions and migrations.
type NodeLibrary struct {
	version int
	nodes   map[string]*NodeDef
	migs    map[string][]Migration
}

// NewNodeLibrary creates an empty node library at version 1.
func NewNodeLibrary() *NodeLibrary {
	return &NodeLibrary{version: 1, nodes: map[string]*NodeDef{}, migs: map[string][]Migration{}}
}

// Version returns the library version.
func (l *NodeLibrary) Version() int { return l.version }

// SetVersion sets the library version. Used when the node library is upgraded.
func (l *NodeLibrary) SetVersion(v int) { l.version = v }

// Register adds a node definition to the library. If a definition with the
// same type already exists, it is replaced.
func (l *NodeLibrary) Register(def *NodeDef) {
	if l.nodes == nil {
		l.nodes = map[string]*NodeDef{}
	}
	l.nodes[def.Type] = def
}

// RegisterMigration registers a migration between two versions of a node type.
func (l *NodeLibrary) RegisterMigration(m Migration) {
	if l.migs == nil {
		l.migs = map[string][]Migration{}
	}
	l.migs[m.NodeType] = append(l.migs[m.NodeType], m)
}

// Get returns the node definition for the given type and whether it exists.
func (l *NodeLibrary) Get(typ string) (*NodeDef, bool) {
	d, ok := l.nodes[typ]
	return d, ok
}

// Migrations returns the registered migrations for a node type, sorted by
// source version.
func (l *NodeLibrary) Migrations(typ string) []Migration {
	ms := l.migs[typ]
	out := make([]Migration, len(ms))
	copy(out, ms)
	sort.Slice(out, func(i, j int) bool { return out[i].FromVersion < out[j].FromVersion })
	return out
}

// DefaultLibrary returns a library pre-registered with common node types and
// the built-in output node.
func DefaultLibrary() *NodeLibrary {
	l := NewNodeLibrary()
	l.Register(&NodeDef{
		Type:    OutputNodeType,
		Version: 1,
		Inputs: []PortDef{
			{Name: "baseColor", Type: TypeVec3, Default: Vec3Value(0, 0, 0)},
			{Name: "opacity", Type: TypeFloat, Default: FloatValue(1)},
		},
		Outputs: []PortDef{{Name: "result", Type: TypeVec4}},
		Emit: func(c *EmitContext) map[string]string {
			bc := c.InputExpr("baseColor")
			op := c.InputExpr("opacity")
			return map[string]string{"result": "vec4(" + bc + ", " + op + ")"}
		},
		Eval: func(inputs []Value) []Value {
			bc := inputs[0]
			op := inputs[1]
			return []Value{Vec4Value(bc.Vec[0], bc.Vec[1], bc.Vec[2], op.Float)}
		},
	})
	l.Register(&NodeDef{
		Type:    "constant",
		Version: 1,
		Inputs:  []PortDef{{Name: "value", Type: TypeFloat, Default: FloatValue(0)}},
		Outputs: []PortDef{{Name: "out", Type: TypeFloat}},
		Emit: func(c *EmitContext) map[string]string {
			return map[string]string{"out": c.InputExpr("value")}
		},
		Eval: func(inputs []Value) []Value {
			return []Value{inputs[0]}
		},
	})
	l.Register(&NodeDef{
		Type:    "add",
		Version: 1,
		Inputs:  []PortDef{{Name: "a", Type: TypeFloat, Default: FloatValue(0)}, {Name: "b", Type: TypeFloat, Default: FloatValue(0)}},
		Outputs: []PortDef{{Name: "out", Type: TypeFloat}},
		Emit: func(c *EmitContext) map[string]string {
			return map[string]string{"out": c.InputExpr("a") + " + " + c.InputExpr("b")}
		},
		Eval: func(inputs []Value) []Value {
			return []Value{FloatValue(inputs[0].Float + inputs[1].Float)}
		},
	})
	l.Register(&NodeDef{
		Type:    "subtract",
		Version: 1,
		Inputs:  []PortDef{{Name: "a", Type: TypeFloat, Default: FloatValue(0)}, {Name: "b", Type: TypeFloat, Default: FloatValue(0)}},
		Outputs: []PortDef{{Name: "out", Type: TypeFloat}},
		Emit: func(c *EmitContext) map[string]string {
			return map[string]string{"out": c.InputExpr("a") + " - " + c.InputExpr("b")}
		},
		Eval: func(inputs []Value) []Value {
			return []Value{FloatValue(inputs[0].Float - inputs[1].Float)}
		},
	})
	l.Register(&NodeDef{
		Type:    "multiply",
		Version: 1,
		Inputs:  []PortDef{{Name: "a", Type: TypeFloat, Default: FloatValue(1)}, {Name: "b", Type: TypeFloat, Default: FloatValue(1)}},
		Outputs: []PortDef{{Name: "out", Type: TypeFloat}},
		Emit: func(c *EmitContext) map[string]string {
			return map[string]string{"out": c.InputExpr("a") + " * " + c.InputExpr("b")}
		},
		Eval: func(inputs []Value) []Value {
			return []Value{FloatValue(inputs[0].Float * inputs[1].Float)}
		},
	})
	l.Register(&NodeDef{
		Type:    "mix",
		Version: 1,
		Inputs:  []PortDef{{Name: "a", Type: TypeFloat, Default: FloatValue(0)}, {Name: "b", Type: TypeFloat, Default: FloatValue(1)}, {Name: "t", Type: TypeFloat, Default: FloatValue(0.5)}},
		Outputs: []PortDef{{Name: "out", Type: TypeFloat}},
		Emit: func(c *EmitContext) map[string]string {
			return map[string]string{"out": "mix(" + c.InputExpr("a") + ", " + c.InputExpr("b") + ", " + c.InputExpr("t") + ")"}
		},
		Eval: func(inputs []Value) []Value {
			a, b, t := inputs[0].Float, inputs[1].Float, inputs[2].Float
			return []Value{FloatValue(a + (b-a)*t)}
		},
	})
	l.Register(&NodeDef{
		Type:    "clamp",
		Version: 1,
		Inputs:  []PortDef{{Name: "x", Type: TypeFloat, Default: FloatValue(0)}, {Name: "min", Type: TypeFloat, Default: FloatValue(0)}, {Name: "max", Type: TypeFloat, Default: FloatValue(1)}},
		Outputs: []PortDef{{Name: "out", Type: TypeFloat}},
		Emit: func(c *EmitContext) map[string]string {
			return map[string]string{"out": "clamp(" + c.InputExpr("x") + ", " + c.InputExpr("min") + ", " + c.InputExpr("max") + ")"}
		},
		Eval: func(inputs []Value) []Value {
			x, mn, mx := inputs[0].Float, inputs[1].Float, inputs[2].Float
			if x < mn {
				x = mn
			}
			if x > mx {
				x = mx
			}
			return []Value{FloatValue(x)}
		},
	})
	l.Register(&NodeDef{
		Type:    "split",
		Version: 1,
		Inputs:  []PortDef{{Name: "in", Type: TypeVec3, Default: Vec3Value(0, 0, 0)}},
		Outputs: []PortDef{{Name: "x", Type: TypeFloat}, {Name: "y", Type: TypeFloat}, {Name: "z", Type: TypeFloat}},
		Emit: func(c *EmitContext) map[string]string {
			in := c.InputExpr("in")
			return map[string]string{"x": in + ".x", "y": in + ".y", "z": in + ".z"}
		},
		Eval: func(inputs []Value) []Value {
			v := inputs[0]
			return []Value{FloatValue(v.Vec[0]), FloatValue(v.Vec[1]), FloatValue(v.Vec[2])}
		},
	})
	l.Register(&NodeDef{
		Type:    "combine",
		Version: 1,
		Inputs:  []PortDef{{Name: "x", Type: TypeFloat, Default: FloatValue(0)}, {Name: "y", Type: TypeFloat, Default: FloatValue(0)}, {Name: "z", Type: TypeFloat, Default: FloatValue(0)}},
		Outputs: []PortDef{{Name: "out", Type: TypeVec3}},
		Emit: func(c *EmitContext) map[string]string {
			return map[string]string{"out": "vec3(" + c.InputExpr("x") + ", " + c.InputExpr("y") + ", " + c.InputExpr("z") + ")"}
		},
		Eval: func(inputs []Value) []Value {
			return []Value{Vec3Value(inputs[0].Float, inputs[1].Float, inputs[2].Float)}
		},
	})
	return l
}
