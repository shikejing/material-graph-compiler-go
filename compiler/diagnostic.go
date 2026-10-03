package compiler

import "strings"

// Severity classifies a diagnostic.
type Severity int

const (
	SeverityError Severity = iota
	SeverityWarning
	SeverityInfo
)

func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	case SeverityInfo:
		return "info"
	}
	return "unknown"
}

// DiagnosticKind identifies the kind of diagnostic.
type DiagnosticKind string

const (
	DiagCycle           DiagnosticKind = "cycle"
	DiagPortNotFound    DiagnosticKind = "port_not_found"
	DiagTypeMismatch    DiagnosticKind = "type_mismatch"
	DiagUnknownNodeType DiagnosticKind = "unknown_node_type"
	DiagMigrationFailed DiagnosticKind = "migration_failed"
	DiagInvalidParam    DiagnosticKind = "invalid_parameter"
	DiagDuplicateNode   DiagnosticKind = "duplicate_node"
	DiagNoOutput        DiagnosticKind = "no_output"
	DiagMultipleOutputs DiagnosticKind = "multiple_outputs"
	DiagOutputNotFound  DiagnosticKind = "output_not_found"
	DiagEliminated      DiagnosticKind = "eliminated"
	DiagFolded          DiagnosticKind = "folded"
	DiagReused          DiagnosticKind = "reused"
	DiagRecompiled      DiagnosticKind = "recompiled"
)

// Diagnostic is a locatable, structured compiler diagnostic.
type Diagnostic struct {
	Severity Severity
	Kind     DiagnosticKind
	NodeID   string
	Port     string
	Message  string
}

func (d Diagnostic) Error() string {
	var b strings.Builder
	b.WriteString(d.Severity.String())
	b.WriteByte(' ')
	b.WriteString(string(d.Kind))
	if d.NodeID != "" {
		b.WriteString(" node=")
		b.WriteString(d.NodeID)
	}
	if d.Port != "" {
		b.WriteString(" port=")
		b.WriteString(d.Port)
	}
	if d.Message != "" {
		b.WriteString(": ")
		b.WriteString(d.Message)
	}
	return b.String()
}

// HasError reports whether any diagnostic is an error.
func HasError(diags []Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == SeverityError {
			return true
		}
	}
	return false
}
