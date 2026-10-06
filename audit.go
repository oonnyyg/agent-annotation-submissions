package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// RequestReport describes one fully parsed request.
type RequestReport struct {
	Index       int    `json:"index"`
	StartOffset int    `json:"startOffset"`
	EndOffset   int    `json:"endOffset"` // offset of the first byte after this request
	Method      string `json:"method"`
	Target      string `json:"target"`
	BodyLength  int    `json:"bodyLength"` // decoded body length
	BodySHA256  string `json:"bodySha256"`
}

// AuditError reports a framing violation at a raw byte offset.
type AuditError struct {
	Offset  int    `json:"offset"`
	Message string `json:"message"`
}

func (e *AuditError) Error() string {
	return fmt.Sprintf("offset %d: %s", e.Offset, e.Message)
}

// AuditResult is the top-level JSON report.
type AuditResult struct {
	File          string          `json:"file"`
	TotalBytes    int             `json:"totalBytes"`
	RequestCount  int             `json:"requestCount"`
	Requests      []RequestReport `json:"requests"`
	Error         *AuditError     `json:"error,omitempty"`
	TrailingBytes int             `json:"trailingBytes"` // bytes after last complete request
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

func isValidToken(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isTokenChar(s[i]) {
			return false
		}
	}
	return true
}

// findCRLF returns the index of the next CRLF at or after pos, or -1.
func findCRLF(data []byte, pos int) int {
	for i := pos; i+1 < len(data); i++ {
		if data[i] == '\r' && data[i+1] == '\n' {
			return i
		}
	}
	return -1
}

// header is a single parsed header field.
type header struct {
	name   string // lower-cased field name
	value  string // trimmed field value
	offset int    // raw offset of the header line
}

// parseRequestLine validates "METHOD SP /target SP HTTP/1.1 CRLF".
// Returns method, target and the offset just past the CRLF.
func parseRequestLine(data []byte, pos int) (string, string, int, error) {
	end := findCRLF(data, pos)
	if end < 0 {
		return "", "", 0, &AuditError{Offset: pos, Message: "truncated request line: no CRLF found"}
	}
	line := string(data[pos:end])
	// Exactly two spaces: method SP target SP version.
	sp1 := strings.IndexByte(line, ' ')
	if sp1 <= 0 {
		return "", "", 0, &AuditError{Offset: pos, Message: "request line missing method or space separator"}
	}
	rest := line[sp1+1:]
	sp2 := strings.IndexByte(rest, ' ')
	if sp2 <= 0 {
		return "", "", 0, &AuditError{Offset: pos + sp1 + 1, Message: "request line missing target or version"}
	}
	method, target, version := line[:sp1], rest[:sp2], rest[sp2+1:]
	if !isValidToken(method) {
		return "", "", 0, &AuditError{Offset: pos, Message: fmt.Sprintf("invalid method token %q", method)}
	}
	if !strings.HasPrefix(target, "/") {
		return "", "", 0, &AuditError{Offset: pos + sp1 + 1, Message: fmt.Sprintf("request target %q must start with '/'", target)}
	}
	if strings.ContainsAny(target, " \t\r\n") {
		return "", "", 0, &AuditError{Offset: pos + sp1 + 1, Message: "request target contains whitespace"}
	}
	if version != "HTTP/1.1" {
		return "", "", 0, &AuditError{Offset: pos + sp1 + 1 + sp2 + 1, Message: fmt.Sprintf("unsupported HTTP version %q", version)}
	}
	return method, target, end + 2, nil
}

// parseHeaders reads header fields up to and including the terminating empty
// line. Returns the fields and the offset just past the blank-line CRLF.
func parseHeaders(data []byte, pos int) ([]header, int, error) {
	var headers []header
	for {
		if pos >= len(data) {
			return nil, 0, &AuditError{Offset: pos, Message: "truncated headers: no terminating empty line"}
		}
		// Empty line ends the header section.
		if pos+1 < len(data) && data[pos] == '\r' && data[pos+1] == '\n' {
			return headers, pos + 2, nil
		}
		// Reject obs-fold (line starting with SP/HTAB).
		if data[pos] == ' ' || data[pos] == '\t' {
			return nil, 0, &AuditError{Offset: pos, Message: "folded (continued) header line is not allowed"}
		}
		end := findCRLF(data, pos)
		if end < 0 {
			return nil, 0, &AuditError{Offset: pos, Message: "truncated header line: no CRLF found"}
		}
		line := string(data[pos:end])
		colon := strings.IndexByte(line, ':')
		if colon <= 0 {
			return nil, 0, &AuditError{Offset: pos, Message: "header line missing colon separator"}
		}
		name := line[:colon]
		// Reject whitespace before the colon and validate the field name.
		if name != strings.TrimRight(name, " \t") {
			return nil, 0, &AuditError{Offset: pos + colon - 1, Message: "whitespace before header colon is not allowed"}
		}
		if !isValidToken(name) {
			return nil, 0, &AuditError{Offset: pos, Message: fmt.Sprintf("invalid header field name %q", name)}
		}
		value := strings.Trim(line[colon+1:], " \t")
		headers = append(headers, header{name: strings.ToLower(name), value: value, offset: pos})
		pos = end + 2
	}
}

// framingInfo extracts body framing rules from validated headers.
func framingInfo(headers []header) (useChunked bool, length int64, err error) {
	var clValues []string
	var teValues []string
	clOffset, teOffset, hostOffset := 0, 0, 0
	hostCount := 0
	for _, h := range headers {
		switch h.name {
		case "host":
			hostCount++
			hostOffset = h.offset
			if h.value == "" {
				return false, 0, &AuditError{Offset: h.offset, Message: "Host header must be non-empty"}
			}
		case "content-length":
			clValues = append(clValues, h.value)
			clOffset = h.offset
		case "transfer-encoding":
			teValues = append(teValues, h.value)
			teOffset = h.offset
		}
	}
	if hostCount != 1 {
		return false, 0, &AuditError{Offset: hostOffset, Message: fmt.Sprintf("exactly one Host header required, got %d", hostCount)}
	}
	if len(clValues) > 1 {
		return false, 0, &AuditError{Offset: clOffset, Message: "duplicate Content-Length headers are not allowed"}
	}
	if len(clValues) == 1 {
		v := clValues[0]
		if strings.Contains(v, ",") {
			return false, 0, &AuditError{Offset: clOffset, Message: "Content-Length list is not allowed"}
		}
		if v == "" {
			return false, 0, &AuditError{Offset: clOffset, Message: "empty Content-Length value"}
		}
		for i := 0; i < len(v); i++ {
			if v[i] < '0' || v[i] > '9' {
				return false, 0, &AuditError{Offset: clOffset, Message: fmt.Sprintf("invalid Content-Length %q: not a decimal integer", v)}
			}
		}
		n, convErr := strconv.ParseInt(v, 10, 63)
		if convErr != nil {
			return false, 0, &AuditError{Offset: clOffset, Message: fmt.Sprintf("invalid Content-Length %q", v)}
		}
		length = n
	}
	if len(teValues) > 0 {
		if len(clValues) > 0 {
			return false, 0, &AuditError{Offset: teOffset, Message: "Content-Length with Transfer-Encoding is not allowed"}
		}
		if len(teValues) != 1 || !strings.EqualFold(teValues[0], "chunked") {
			return false, 0, &AuditError{Offset: teOffset, Message: "Transfer-Encoding must be a single 'chunked'"}
		}
		useChunked = true
	}
	return useChunked, length, nil
}

// parseChunkedBody decodes a chunked body starting at pos. It returns the
// decoded body and the offset just past the terminating empty line after
// trailers. Chunk-size lines, CRLFs and trailers are excluded from the body.
func parseChunkedBody(data []byte, pos int) ([]byte, int, error) {
	var body []byte
	for {
		end := findCRLF(data, pos)
		if end < 0 {
			return nil, 0, &AuditError{Offset: pos, Message: "truncated chunk-size line"}
		}
		sizeText := string(data[pos:end])
		if strings.Contains(sizeText, ";") {
			return nil, 0, &AuditError{Offset: pos, Message: "chunk extensions are not allowed"}
		}
		if sizeText == "" {
			return nil, 0, &AuditError{Offset: pos, Message: "empty chunk-size line"}
		}
		size, err := strconv.ParseInt(sizeText, 16, 63)
		if err != nil {
			return nil, 0, &AuditError{Offset: pos, Message: fmt.Sprintf("invalid chunk size %q", sizeText)}
		}
		pos = end + 2
		if size == 0 {
			// Trailer section: header fields until an empty line.
			trailers, next, terr := parseHeaders(data, pos)
			if terr != nil {
				return nil, 0, terr
			}
			for _, t := range trailers {
				switch t.name {
				case "content-length", "transfer-encoding", "host":
					return nil, 0, &AuditError{Offset: t.offset, Message: fmt.Sprintf("forbidden trailer field %q", t.name)}
				}
			}
			return body, next, nil
		}
		if int64(len(data)-pos) < size+2 {
			return nil, 0, &AuditError{Offset: pos, Message: "truncated chunk data"}
		}
		body = append(body, data[pos:pos+int(size)]...)
		pos += int(size)
		if data[pos] != '\r' || data[pos+1] != '\n' {
			return nil, 0, &AuditError{Offset: pos, Message: "missing CRLF after chunk data"}
		}
		pos += 2
	}
}

// Audit parses data as a sequence of strict HTTP/1.1 requests. On the first
// framing error it stops; previously completed requests are kept and no
// resynchronization is attempted.
func Audit(fileName string, data []byte) *AuditResult {
	result := &AuditResult{File: fileName, TotalBytes: len(data)}
	pos := 0
	for pos < len(data) {
		start := pos
		method, target, next, err := parseRequestLine(data, pos)
		if err != nil {
			result.Error = err.(*AuditError)
			break
		}
		headers, next, err := parseHeaders(data, next)
		if err != nil {
			result.Error = err.(*AuditError)
			break
		}
		chunked, length, ferr := framingInfo(headers)
		if ferr != nil {
			result.Error = ferr.(*AuditError)
			break
		}
		var body []byte
		if chunked {
			var berr error
			body, next, berr = parseChunkedBody(data, next)
			if berr != nil {
				result.Error = berr.(*AuditError)
				break
			}
		} else if length > 0 {
			if int64(len(data)-next) < length {
				result.Error = &AuditError{Offset: next, Message: fmt.Sprintf("truncated body: need %d bytes, have %d", length, len(data)-next)}
				break
			}
			body = data[next : next+int(length)]
			next += int(length)
		}
		sum := sha256.Sum256(body)
		result.Requests = append(result.Requests, RequestReport{
			Index:       len(result.Requests),
			StartOffset: start,
			EndOffset:   next,
			Method:      method,
			Target:      target,
			BodyLength:  len(body),
			BodySHA256:  hex.EncodeToString(sum[:]),
		})
		pos = next
	}
	result.RequestCount = len(result.Requests)
	if len(result.Requests) > 0 {
		result.TrailingBytes = len(data) - result.Requests[len(result.Requests)-1].EndOffset
	} else {
		result.TrailingBytes = len(data)
	}
	return result
}
