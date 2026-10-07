package dnswire

import "fmt"

// WireError is a structured error carrying the byte offset in the message
// (or JSON document, for encode-side failures) where the problem was found.
type WireError struct {
	Offset int    `json:"offset"`
	Reason string `json:"reason"`
}

func (e *WireError) Error() string {
	return fmt.Sprintf("offset %d: %s", e.Offset, e.Reason)
}

func errAt(offset int, format string, args ...any) *WireError {
	return &WireError{Offset: offset, Reason: fmt.Sprintf(format, args...)}
}
