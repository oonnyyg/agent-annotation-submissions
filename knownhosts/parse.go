// Package knownhosts parses OpenSSH known_hosts samples and audits host keys
// offline. It never opens network connections and never touches the user's
// real key directories.
package knownhosts

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// LineKind classifies a parsed known_hosts line.
type LineKind int

const (
	KindNormal LineKind = iota
	KindRevoked
	KindCertAuthority // recognized but unsupported
)

// Entry is one parsed known_hosts record.
type Entry struct {
	Line     int
	Kind     LineKind
	Patterns []string // raw comma-separated host patterns
	KeyType  string
	KeyData  []byte // decoded SSH wire-format public key blob
	Comment  string
}

// Diagnostic reports a problem with a sample line. Diagnostics never
// produce credentials; they only mark the audit incomplete.
type Diagnostic struct {
	Line   int    `json:"line"`
	Reason string `json:"reason"`
}

func (d Diagnostic) Error() string {
	return fmt.Sprintf("line %d: %s", d.Line, d.Reason)
}

// Sample is a parsed known_hosts file.
type Sample struct {
	Entries     []Entry
	Diagnostics []Diagnostic
}

// Parse parses known_hosts content. Malformed lines become diagnostics and
// are never treated as valid credentials.
func Parse(content string) *Sample {
	s := &Sample{}
	lines := strings.Split(content, "\n")
	for i, raw := range lines {
		lineNo := i + 1
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		entry, err := parseLine(lineNo, line)
		if err != nil {
			s.Diagnostics = append(s.Diagnostics, Diagnostic{Line: lineNo, Reason: err.Error()})
			continue
		}
		s.Entries = append(s.Entries, *entry)
	}
	return s
}

func parseLine(lineNo int, line string) (*Entry, error) {
	e := &Entry{Line: lineNo, Kind: KindNormal}
	if strings.HasPrefix(line, "@") {
		marker, rest, _ := strings.Cut(line, " ")
		switch marker {
		case "@revoked":
			e.Kind = KindRevoked
		case "@cert-authority":
			e.Kind = KindCertAuthority
		default:
			return nil, fmt.Errorf("unsupported marker %q", marker)
		}
		line = strings.TrimSpace(rest)
		if line == "" {
			return nil, fmt.Errorf("marker %s without record", marker)
		}
	}
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return nil, fmt.Errorf("expected at least 3 fields (hosts, key-type, key), got %d", len(fields))
	}
	e.Patterns = strings.Split(fields[0], ",")
	for _, p := range e.Patterns {
		if p == "" {
			return nil, fmt.Errorf("empty host pattern in list %q", fields[0])
		}
	}
	e.KeyType = fields[1]
	blob, err := base64.StdEncoding.DecodeString(fields[2])
	if err != nil {
		return nil, fmt.Errorf("invalid base64 key data: %v", err)
	}
	e.KeyData = blob
	if len(fields) > 3 {
		e.Comment = strings.Join(fields[3:], " ")
	}
	return e, nil
}
