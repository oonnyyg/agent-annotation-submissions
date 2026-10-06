// Package auditor implements an offline HTTP/1.1 request boundary auditor.
// It parses a strict subset of HTTP/1.1 from a byte stream and reports
// per-request byte offsets and body digests. It is not an HTTP server.
package auditor

import (
	"errors"
	"fmt"
)

// Request describes one successfully parsed request.
type Request struct {
	Index       int    `json:"index"`
	StartOffset int64  `json:"start_offset"` // offset of first byte of request line
	EndOffset   int64  `json:"end_offset"`   // offset one past the last byte of the request
	Method      string `json:"method"`
	Target      string `json:"target"`
	BodyLength  int64  `json:"body_length"` // decoded body length
	BodySHA256  string `json:"body_sha256"` // hex digest of decoded body
}

// AuditError reports a parse failure at a specific raw offset.
type AuditError struct {
	Offset  int64  `json:"offset"`
	Message string `json:"message"`
}

func (e *AuditError) Error() string {
	return fmt.Sprintf("offset %d: %s", e.Offset, e.Message)
}

// Report is the result of auditing a byte stream.
type Report struct {
	Requests []Request   `json:"requests"`
	Error    *AuditError `json:"error,omitempty"`
}

func isTokenChar(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	}
	switch c {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	}
	return false
}

func isWhitespace(c byte) bool { return c == ' ' || c == '\t' }

func errAt(off int64, format string, args ...any) *AuditError {
	return &AuditError{Offset: off, Message: fmt.Sprintf(format, args...)}
}

var errNeedMore = errors.New("auditor: truncated input")
