package tomledit

import "fmt"

// ParseError represents a lexing or parsing error with position information.
// Line and Column are 1-based. Snippet may contain a fragment of the source
// near the error for diagnostic purposes.
type ParseError struct {
	Line    int
	Column  int
	Offset  int
	Snippet string
	Message string
}

// Error returns the formatted error message with line and column information.
func (e *ParseError) Error() string {
	return fmt.Sprintf("line %d, column %d: %s", e.Line, e.Column, e.Message)
}
