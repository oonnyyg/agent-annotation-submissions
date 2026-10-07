package main

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// Verdict values for a query.
const (
	VerdictTrusted     = "trusted"
	VerdictKeyChanged  = "key-changed"
	VerdictFirstSeen   = "first-seen"
	VerdictRevoked     = "revoked"
	VerdictUnsupported = "unsupported"
)

// MatchRef references one known_hosts line relevant to a query.
type MatchRef struct {
	Line    int    `json:"line"`
	Kind    string `json:"kind"`
	KeyType string `json:"keyType"`
	Comment string `json:"comment,omitempty"`
}

// Result is the audit outcome for one query.
type Result struct {
	Host        string     `json:"host"`
	Port        int        `json:"port"`
	Fingerprint string     `json:"fingerprint,omitempty"`
	Verdict     string     `json:"verdict"`
	Reason      string     `json:"reason"`
	Matches     []MatchRef `json:"matches,omitempty"`
}

// fingerprint computes the SHA256 fingerprint over the full SSH wire
// encoding of the key, per OpenSSH convention.
func fingerprint(keyB64 string) string {
	blob, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(blob)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

// audit checks a presented key against all entries for the target.
// Revocation is checked across the whole file before trusting any
// normal match; negated patterns only exclude their own line.
func audit(f *File, t Target, keyType, keyB64 string) Result {
	res := Result{Host: t.Host, Port: t.Port}
	if keyType != "ssh-ed25519" {
		res.Verdict = VerdictUnsupported
		res.Reason = fmt.Sprintf("presented key type %q is not supported (only ssh-ed25519)", keyType)
		return res
	}
	if _, err := decodeKey(Entry{KeyType: keyType, KeyB64: keyB64}); err != nil {
		res.Verdict = VerdictUnsupported
		res.Reason = "presented key is malformed: " + err.Error()
		return res
	}
	res.Fingerprint = fingerprint(keyB64)

	var trusted, revoked, different []MatchRef
	for _, e := range f.Entries {
		if e.Unsupported || !hostsFieldMatches(e.Hosts, t) {
			continue
		}
		if _, err := decodeKey(e); err != nil {
			continue // corrupt entry: reported in diagnostics, never a credential
		}
		ref := MatchRef{Line: e.Line, KeyType: e.KeyType, Comment: e.Comment}
		sameKey := e.KeyType == keyType && e.KeyB64 == keyB64
		switch {
		case e.Revoked && sameKey:
			ref.Kind = "revoked"
			revoked = append(revoked, ref)
		case !e.Revoked && sameKey:
			ref.Kind = "trusted"
			trusted = append(trusted, ref)
		default:
			ref.Kind = "different-key"
			different = append(different, ref)
		}
	}

	switch {
	case len(revoked) > 0:
		res.Verdict = VerdictRevoked
		res.Reason = "key appears on a @revoked line; rejecting regardless of other matches"
		res.Matches = append(revoked, append(trusted, different...)...)
	case len(trusted) > 0:
		res.Verdict = VerdictTrusted
		res.Reason = "key matches a known_hosts entry"
		res.Matches = append(trusted, different...)
	case len(different) > 0:
		res.Verdict = VerdictKeyChanged
		res.Reason = "host is known but the presented key differs from all recorded keys"
		res.Matches = different
	default:
		res.Verdict = VerdictFirstSeen
		res.Reason = "no known_hosts entry matches this host and port"
	}
	return res
}
