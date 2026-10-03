package graph

// Diagnostic 是一条可定位到节点与端口的诊断信息。
type Diagnostic struct {
	Code    string // 稳定的诊断码，如 CYCLE / TYPE_MISMATCH / PORT_NOT_FOUND
	Message string
	Node    NodeID // 相关节点（可为空）
	Port    PortID // 相关端口（可为空）
}

// Error 是整图校验失败时返回的错误，携带全部诊断。
type Error struct {
	Diags []Diagnostic
}

func (e *Error) Error() string {
	if len(e.Diags) == 0 {
		return "graph validation failed"
	}
	return "graph validation failed: " + e.Diags[0].Message
}
