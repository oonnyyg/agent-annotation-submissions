package auditor

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

// parseContentLength validates a Content-Length value: a single non-negative
// decimal integer, no list syntax.
func parseContentLength(v string, off int64) (int64, *AuditError) {
	if v == "" {
		return 0, errAt(off, "empty Content-Length value")
	}
	if strings.Contains(v, ",") {
		return 0, errAt(off, "Content-Length list is not allowed")
	}
	for i := 0; i < len(v); i++ {
		if v[i] < '0' || v[i] > '9' {
			return 0, errAt(off, "Content-Length must be a non-negative decimal integer")
		}
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, errAt(off, "Content-Length out of range")
	}
	return n, nil
}

// readFixedBody reads exactly n bytes of body starting at p.pos.
func (p *parser) readFixedBody(n int64) ([]byte, *AuditError) {
	if n < 0 || p.pos+n > int64(len(p.data)) {
		return nil, errAt(p.pos, "truncated body: need %d bytes", n)
	}
	body := p.data[p.pos : p.pos+n]
	p.pos += n
	return body, nil
}

// readChunkedBody decodes a chunked body, rejecting chunk extensions and
// forbidden trailer fields. Returns the decoded body.
func (p *parser) readChunkedBody() ([]byte, *AuditError) {
	var body []byte
	for {
		lineOff := p.pos
		line, err := p.readLine()
		if err != nil {
			return nil, err
		}
		if strings.ContainsAny(line, ";") {
			return nil, errAt(lineOff, "chunk extensions are not allowed")
		}
		size, cerr := strconv.ParseInt(line, 16, 64)
		if cerr != nil || size < 0 || line == "" || line[0] == '+' || line[0] == '-' {
			return nil, errAt(lineOff, "invalid chunk size %q", line)
		}
		if size == 0 {
			// Trailer section: field lines until empty line.
			for {
				tOff := p.pos
				tLine, terr := p.readLine()
				if terr != nil {
					return nil, terr
				}
				if tLine == "" {
					return body, nil
				}
				h, herr := parseHeaderLine(tLine, tOff)
				if herr != nil {
					return nil, herr
				}
				switch h.name {
				case "content-length", "transfer-encoding", "host":
					return nil, errAt(tOff, "forbidden trailer field %q", h.name)
				}
			}
		}
		if p.pos+size > int64(len(p.data)) {
			return nil, errAt(p.pos, "truncated chunk data: need %d bytes", size)
		}
		body = append(body, p.data[p.pos:p.pos+size]...)
		p.pos += size
		if p.pos+2 > int64(len(p.data)) {
			return nil, errAt(p.pos, "truncated chunk data CRLF")
		}
		if p.data[p.pos] != '\r' || p.data[p.pos+1] != '\n' {
			return nil, errAt(p.pos, "chunk data not followed by CRLF")
		}
		p.pos += 2
	}
}

func digest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
