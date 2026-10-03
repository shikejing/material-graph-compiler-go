package graph

import (
	"fmt"
	"strconv"
	"strings"
)

// Canonical 返回值的规范字符串形式，用于确定性输出与哈希。
func (v Value) Canonical() string {
	switch v.Type {
	case TypeFloat:
		return "float(" + formatFloat(v.Float) + ")"
	case TypeInt:
		return "int(" + strconv.FormatInt(v.Int, 10) + ")"
	case TypeBool:
		return "bool(" + strconv.FormatBool(v.Bool) + ")"
	case TypeVec2, TypeVec3, TypeVec4, TypeColor:
		parts := make([]string, len(v.Vec))
		for i, f := range v.Vec {
			parts[i] = formatFloat(f)
		}
		return string(v.Type) + "(" + strings.Join(parts, ",") + ")"
	case TypeTexture:
		return "texture(" + v.Texture + ")"
	}
	return "unknown"
}

// formatFloat 以确定方式格式化浮点数。
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// FloatValue 构造 float 常量。
func FloatValue(f float64) Value { return Value{Type: TypeFloat, Float: f} }

// VecValue 构造向量常量。
func VecValue(t ValueType, comps ...float64) Value {
	return Value{Type: t, Vec: append([]float64(nil), comps...)}
}

// Convert 把值按 Compatible 规则转换为目标类型；不可转换时 ok=false。
func Convert(v Value, to ValueType) (Value, bool) {
	if v.Type == to {
		return v, true
	}
	switch v.Type {
	case TypeFloat:
		switch to {
		case TypeVec2:
			return VecValue(to, v.Float, v.Float), true
		case TypeVec3:
			return VecValue(to, v.Float, v.Float, v.Float), true
		case TypeVec4, TypeColor:
			return VecValue(to, v.Float, v.Float, v.Float, v.Float), true
		}
	case TypeInt:
		if to == TypeFloat {
			return FloatValue(float64(v.Int)), true
		}
	case TypeVec3:
		if to == TypeVec4 || to == TypeColor {
			return VecValue(to, v.Vec[0], v.Vec[1], v.Vec[2], 1), true
		}
	}
	return Value{}, false
}

// Literal 把常量渲染为着色语言字面量（确定性）。
func (v Value) Literal() string {
	switch v.Type {
	case TypeFloat:
		return formatFloat(v.Float)
	case TypeInt:
		return strconv.FormatInt(v.Int, 10)
	case TypeBool:
		return strconv.FormatBool(v.Bool)
	case TypeVec2, TypeVec3, TypeVec4, TypeColor:
		typ := string(v.Type)
		if v.Type == TypeColor {
			typ = "vec4"
		}
		parts := make([]string, len(v.Vec))
		for i, f := range v.Vec {
			parts[i] = formatFloat(f)
		}
		return fmt.Sprintf("%s(%s)", typ, strings.Join(parts, ", "))
	case TypeTexture:
		return fmt.Sprintf("texture(%q)", v.Texture)
	}
	return "/* invalid literal */"
}
