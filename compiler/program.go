package compiler

// Program is the compiled shader program: deterministic GLSL source plus
// metadata.
type Program struct {
	GraphKey   string
	LibVersion int
	Code       string
}

// CompileReport describes which nodes were reused or recompiled and which were
// eliminated or folded.
type CompileReport struct {
	Reused     []string
	Recompiled []string
	Eliminated []string
	Folded     []string
}

// CompileResult is the outcome of compiling a single graph.
type CompileResult struct {
	GraphKey    string
	Program     *Program
	Diagnostics []Diagnostic
	Report      *CompileReport
	Success     bool
}
