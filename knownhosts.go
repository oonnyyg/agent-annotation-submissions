package main

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// Entry is one parsed known_hosts line.
type Entry struct {
	Line        int
	Marker      string // "", "@revoked", "@cert-authority"
	Hosts       string // raw host pattern field
	KeyType     string
	KeyB64      string
	Comment     string
	Revoked     bool
	Unsupported bool // key type or marker we deliberately do not trust
}

// Diagnostic is a non-fatal parse problem tied to a line.
type Diagnostic struct {
	Line   int    `json:"line"`
	Reason string `json:"reason"`
	Raw    string `json:"raw"`
}

// File is a parsed known_hosts sample.
type File struct {
	Entries     []Entry
	Diagnostics []Diagnostic
}

// ParseFile parses known_hosts content, keeping line numbers and
// collecting diagnostics instead of silently dropping bad lines.
func ParseFile(content string) *File {
	f := &File{}
	content = strings.TrimPrefix(content, "\ufeff")
	lines := strings.Split(content, "\n")
	for i, raw := range lines {
		lineNo := i + 1
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		e := Entry{Line: lineNo}
		if strings.HasPrefix(fields[0], "@") {
			switch fields[0] {
			case "@revoked":
				e.Marker, e.Revoked = "@revoked", true
			case "@cert-authority":
				e.Marker = "@cert-authority"
				e.Unsupported = true
				f.Diagnostics = append(f.Diagnostics, Diagnostic{Line: lineNo, Reason: "@cert-authority marker is not supported", Raw: line})
			default:
				f.Diagnostics = append(f.Diagnostics, Diagnostic{Line: lineNo, Reason: fmt.Sprintf("unknown marker %q", fields[0]), Raw: line})
				continue
			}
			fields = fields[1:]
		}
		if len(fields) < 3 {
			f.Diagnostics = append(f.Diagnostics, Diagnostic{Line: lineNo, Reason: "expected hosts, key type and base64 key", Raw: line})
			continue
		}
		e.Hosts, e.KeyType, e.KeyB64 = fields[0], fields[1], fields[2]
		if len(fields) > 3 {
			e.Comment = strings.Join(fields[3:], " ")
		}
		if e.KeyType != "ssh-ed25519" {
			e.Unsupported = true
			reason := fmt.Sprintf("key type %q is not supported (only ssh-ed25519)", e.KeyType)
			if strings.Contains(e.KeyType, "-cert-") {
				reason = fmt.Sprintf("certificate key type %q is not supported", e.KeyType)
			}
			f.Diagnostics = append(f.Diagnostics, Diagnostic{Line: lineNo, Reason: reason, Raw: line})
		}
		f.Entries = append(f.Entries, e)
	}
	return f
}

// decodeKey validates the base64 blob and the SSH wire structure for
// ssh-ed25519: string alg || string key(32 bytes).
func decodeKey(e Entry) (rawKey []byte, err error) {
	blob, err := base64.StdEncoding.DecodeString(e.KeyB64)
	if err != nil {
		return nil, fmt.Errorf("invalid base64: %v", err)
	}
	alg, rest, ok := readSSHString(blob)
	if !ok {
		return nil, fmt.Errorf("truncated SSH string for algorithm")
	}
	if string(alg) != e.KeyType {
		return nil, fmt.Errorf("algorithm field %q does not match key type %q", alg, e.KeyType)
	}
	key, rest, ok := readSSHString(rest)
	if !ok {
		return nil, fmt.Errorf("truncated SSH string for key material")
	}
	if len(rest) != 0 {
		return nil, fmt.Errorf("trailing %d bytes after key material", len(rest))
	}
	if e.KeyType == "ssh-ed25519" && len(key) != 32 {
		return nil, fmt.Errorf("ed25519 key must be 32 bytes, got %d", len(key))
	}
	return key, nil
}

func readSSHString(b []byte) (val, rest []byte, ok bool) {
	if len(b) < 4 {
		return nil, nil, false
	}
	n := int(b[0])<<24 | int(b[1])<<16 | int(b[2])<<8 | int(b[3])
	if n < 0 || len(b)-4 < n {
		return nil, nil, false
	}
	return b[4 : 4+n], b[4+n:], true
}
