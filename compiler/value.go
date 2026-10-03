package compiler

import (
	"fmt"
	"strconv"
	"strings"
)

// PortType is the type of a node input/output port.
type PortType string

const (
	TypeFloat   PortType = "float"
	TypeInt     PortType = "int"
	TypeBool    PortType = "bool"
	TypeVec2    PortType = "vec2"
	TypeVec3    PortType = "vec3"
	TypeVec4    PortType = "vec4"
	TypeColor   PortType = "color"
	TypeTexture PortType = "texture"
)

// Value is a constant value held by an input port or produced by folding.
type Value struct {
	Type  PortType
	Float float64
	Int   int64
	Bool  bool
	Vec   [4]float64
	Tex   string
}

func FloatValue(f float64) Value        { return Value{Type: TypeFloat, Float: f} }
func IntValue(i int64) Value           { return Value{Type: TypeInt, Int: i} }
func BoolValue(b bool) Value           { return Value{Type: TypeBool, Bool: b} }
func Vec2Value(x, y float64) Value     { return Value{Type: TypeVec2, Vec: [4]float64{x, y, 0, 0}} }
func Vec3Value(x, y, z float64) Value  { return Value{Type: TypeVec3, Vec: [4]float64{x, y, z, 0}} }
func ColorValue(r, g, b float64) Value { return Value{Type: TypeColor, Vec: [4]float64{r, g, b, 0}} }
func Vec4Value(x, y, z, w float64) Value {
	return Value{Type: TypeVec4, Vec: [4]float64{x, y, z, w}}
}
func TextureValue(name string) Value { return Value{Type: TypeTexture, Tex: name} }

// Compatible reports whether an output port of type `from` may connect to an
// input port of type `to`. The relation is:
//   - identical types are compatible;
//   - color and vec3 are interchangeable (same GLSL representation);
//   - int coerces to float and any vector type;
//   - float coerces to any vector type (splat).
func Compatible(from, to PortType) bool {
	if from == to {
		return true
	}
	if from == TypeColor && to == TypeVec3 {
		return true
	}
	if from == TypeVec3 && to == TypeColor {
		return true
	}
	switch from {
	case TypeInt:
		return to == TypeFloat || to == TypeVec2 || to == TypeVec3 || to == TypeVec4
	case TypeFloat:
		return to == TypeVec2 || to == TypeVec3 || to == TypeVec4
	}
	return false
}

// Coerce converts a constant value to the target type when Compatible allows
// it. It returns false if the conversion is not supported.
func (v Value) Coerce(to PortType) (Value, bool) {
	if v.Type == to {
		return v, true
	}
	if !Compatible(v.Type, to) {
		return Value{}, false
	}
	switch to {
	case TypeFloat:
		switch v.Type {
		case TypeInt:
			return FloatValue(float64(v.Int)), true
		}
	case TypeVec2:
		switch v.Type {
		case TypeInt:
			f := float64(v.Int)
			return Vec2Value(f, f), true
		case TypeFloat:
			return Vec2Value(v.Float, v.Float), true
		}
	case TypeVec3:
		switch v.Type {
		case TypeInt:
			f := float64(v.Int)
			return Vec3Value(f, f, f), true
		case TypeFloat:
			return Vec3Value(v.Float, v.Float, v.Float), true
		case TypeColor:
			return Vec3Value(v.Vec[0], v.Vec[1], v.Vec[2]), true
		}
	case TypeVec4:
		switch v.Type {
		case TypeInt:
			f := float64(v.Int)
			return Vec4Value(f, f, f, f), true
		case TypeFloat:
			return Vec4Value(v.Float, v.Float, v.Float, v.Float), true
		}
	case TypeColor:
		switch v.Type {
		case TypeVec3:
			return ColorValue(v.Vec[0], v.Vec[1], v.Vec[2]), true
		}
	}
	return Value{}, false
}

// ftoa formats a float deterministically without trailing zeros.
func ftoa(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	return s
}

// Canonical returns a deterministic, type-tagged string for hashing.
func (v Value) Canonical() string {
	var b strings.Builder
	b.WriteString(string(v.Type))
	b.WriteByte(':')
	switch v.Type {
	case TypeFloat:
		b.WriteString(ftoa(v.Float))
	case TypeInt:
		b.WriteString(strconv.FormatInt(v.Int, 10))
	case TypeBool:
		if v.Bool {
			b.WriteByte('1')
		} else {
			b.WriteByte('0')
		}
	case TypeVec2, TypeVec3, TypeVec4, TypeColor:
		n := 2
		if v.Type == TypeVec3 || v.Type == TypeColor {
			n = 3
		} else if v.Type == TypeVec4 {
			n = 4
		}
		for i := 0; i < n; i++ {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(ftoa(v.Vec[i]))
		}
	case TypeTexture:
		b.WriteString(v.Tex)
	default:
		b.WriteString(fmt.Sprintf("%v", v))
	}
	return b.String()
}

// GLSL returns the GLSL literal representation of the value.
func (v Value) GLSL() string {
	switch v.Type {
	case TypeFloat:
		return ftoa(v.Float)
	case TypeInt:
		return strconv.FormatInt(v.Int, 10)
	case TypeBool:
		if v.Bool {
			return "true"
		}
		return "false"
	case TypeVec2:
		return "vec2(" + ftoa(v.Vec[0]) + ", " + ftoa(v.Vec[1]) + ")"
	case TypeVec3:
		return "vec3(" + ftoa(v.Vec[0]) + ", " + ftoa(v.Vec[1]) + ", " + ftoa(v.Vec[2]) + ")"
	case TypeColor:
		return "vec3(" + ftoa(v.Vec[0]) + ", " + ftoa(v.Vec[1]) + ", " + ftoa(v.Vec[2]) + ")"
	case TypeVec4:
		return "vec4(" + ftoa(v.Vec[0]) + ", " + ftoa(v.Vec[1]) + ", " + ftoa(v.Vec[2]) + ", " + ftoa(v.Vec[3]) + ")"
	case TypeTexture:
		return "texture(" + v.Tex + ")"
	}
	return "0"
}

// Equal reports whether two values are identical in type and data.
func (v Value) Equal(o Value) bool {
	if v.Type != o.Type {
		return false
	}
	switch v.Type {
	case TypeFloat:
		return v.Float == o.Float
	case TypeInt:
		return v.Int == o.Int
	case TypeBool:
		return v.Bool == o.Bool
	case TypeVec2, TypeVec3, TypeVec4, TypeColor:
		return v.Vec == o.Vec
	case TypeTexture:
		return v.Tex == o.Tex
	}
	return false
}
