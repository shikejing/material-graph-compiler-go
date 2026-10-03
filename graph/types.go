// Package graph 定义材质图的数据模型：节点、端口、连线与图。
package graph

// ValueType 是端口值的类型。
type ValueType string

const (
	TypeFloat  ValueType = "float"
	TypeVec2   ValueType = "vec2"
	TypeVec3   ValueType = "vec3"
	TypeVec4   ValueType = "vec4"
	TypeColor  ValueType = "color"
	TypeBool   ValueType = "bool"
	TypeInt    ValueType = "int"
	TypeTexture ValueType = "texture"
)

// Value 是一个常量值，可充当端口默认值。
type Value struct {
	Type   ValueType
	Float  float64
	Int    int64
	Bool   bool
	Vec    []float64 // 用于 vec2/vec3/vec4/color
	Texture string
}

// PortID 在节点内唯一标识一个端口。
type PortID string

// NodeID 在图内唯一标识一个节点。
type NodeID string

// PortRef 指向某节点的某个端口。
type PortRef struct {
	Node NodeID
	Port PortID
}

// Node 是图中的一个节点实例。
type Node struct {
	ID        NodeID
	Type      string            // 节点库中的节点类型名
	Overrides map[PortID]*Value // 输入端口的覆盖值（未连线时生效）
}

// Edge 是一条从输出端口到输入端口的连线。
type Edge struct {
	From PortRef // 输出端
	To   PortRef // 输入端
}

// Graph 是一张材质图。
type Graph struct {
	Nodes      []Node
	Edges      []Edge
	OutputNode NodeID // 产出最终材质的输出节点
	OutputPort PortID
}
