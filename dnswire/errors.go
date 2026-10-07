package dnswire

import "fmt"

// WireError describes a structured decode/encode failure with the byte
// offset (relative to the message start) where it was detected.
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
