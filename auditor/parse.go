package auditor

import (
	"strings"
)

// parser walks a byte slice with an absolute position.
type parser struct {
	data []byte
	pos  int64
}

// readLine reads one line terminated by CRLF. Bare CR or LF is rejected.
// Returns the line content without the CRLF.
func (p *parser) readLine() (string, *AuditError) {
	start := p.pos
	for {
		if p.pos >= int64(len(p.data)) {
			return "", errAt(p.pos, "unexpected end of input: %v", errNeedMore)
		}
		c := p.data[p.pos]
		switch c {
		case '\r':
			if p.pos+1 >= int64(len(p.data)) {
				return "", errAt(p.pos, "unexpected end of input after CR")
			}
			if p.data[p.pos+1] != '\n' {
				return "", errAt(p.pos, "bare CR not followed by LF")
			}
			line := string(p.data[start:p.pos])
			p.pos += 2
			return line, nil
		case '\n':
			return "", errAt(p.pos, "bare LF where CRLF required")
		default:
			p.pos++
		}
	}
}

// parseRequestLine validates "METHOD SP target SP HTTP/1.1".
func parseRequestLine(line string, off int64) (method, target string, err *AuditError) {
	sp := strings.IndexByte(line, ' ')
	if sp <= 0 {
		return "", "", errAt(off, "malformed request line: missing method or space")
	}
	method = line[:sp]
	for i := 0; i < len(method); i++ {
		if !isTokenChar(method[i]) {
			return "", "", errAt(off+int64(i), "invalid character in method")
		}
	}
	rest := line[sp+1:]
	sp2 := strings.LastIndexByte(rest, ' ')
	if sp2 <= 0 {
		return "", "", errAt(off, "malformed request line: missing target or version")
	}
	target = rest[:sp2]
	version := rest[sp2+1:]
	if version != "HTTP/1.1" {
		return "", "", errAt(off+int64(len(line)-len(version)), "unsupported HTTP version %q", version)
	}
	if !strings.HasPrefix(target, "/") {
		return "", "", errAt(off+int64(sp+1), "request target must start with '/'")
	}
	for i := 0; i < len(target); i++ {
		if isWhitespace(target[i]) {
			return "", "", errAt(off+int64(sp+1+i), "whitespace in request target")
		}
	}
	return method, target, nil
}

// header is a parsed field line.
type header struct {
	name  string // lower-cased
	value string // with optional leading/trailing OWS trimmed
	off   int64  // offset of the field line start
}

// parseHeaderLine validates "field-name ':' OWS value OWS".
func parseHeaderLine(line string, off int64) (header, *AuditError) {
	if line == "" {
		return header{}, errAt(off, "empty header line")
	}
	if isWhitespace(line[0]) {
		return header{}, errAt(off, "obsolete folded header line rejected")
	}
	colon := strings.IndexByte(line, ':')
	if colon <= 0 {
		return header{}, errAt(off, "header line missing colon")
	}
	name := line[:colon]
	for i := 0; i < len(name); i++ {
		if !isTokenChar(name[i]) {
			return header{}, errAt(off+int64(i), "invalid character in header field name")
		}
	}
	if isWhitespace(line[colon-1]) {
		return header{}, errAt(off+int64(colon-1), "whitespace before colon in header field")
	}
	value := strings.Trim(line[colon+1:], " \t")
	return header{name: strings.ToLower(name), value: value, off: off}, nil
}
