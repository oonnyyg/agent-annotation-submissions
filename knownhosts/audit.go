package knownhosts

import (
	"encoding/base64"
	"fmt"
)

// Verdict is the audit conclusion for one query.
type Verdict string

const (
	VerdictTrusted      Verdict = "trusted"      // matched a normal line with the same key
	VerdictRevoked      Verdict = "revoked"      // the same key is revoked; always wins
	VerdictChanged      Verdict = "changed"      // host known, but presented key differs
	VerdictUnknown      Verdict = "unknown"      // first sight: no line matches the target
	VerdictUnsupported  Verdict = "unsupported"  // presented key type is not supported
	VerdictInvalid      Verdict = "invalid"      // presented key is malformed
	VerdictInconclusive Verdict = "inconclusive" // would be trusted, but sample has errors
)

// LineRef cites one matched sample line in a result.
type LineRef struct {
	Line        int    `json:"line"`
	Kind        string `json:"kind"` // "normal", "revoked", "cert-authority"
	Fingerprint string `json:"fingerprint,omitempty"`
	SameKey     bool   `json:"same_key"`
	Note        string `json:"note,omitempty"`
}

// Result is the JSON-serializable audit outcome for one query.
type Result struct {
	Target      string       `json:"target"`
	Fingerprint string       `json:"fingerprint,omitempty"`
	Verdict     Verdict      `json:"verdict"`
	Reason      string       `json:"reason"`
	Lines       []LineRef    `json:"lines,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
	Complete    bool         `json:"complete"`
}

// Query is one audit request: a target plus the presented host key.
type Query struct {
	Target  Target
	KeyType string
	KeyB64  string // base64 SSH wire-format blob, as in known_hosts
}

// Audit evaluates one query against the sample, offline.
func (s *Sample) Audit(q Query) Result {
	res := Result{Target: q.Target.String(), Complete: len(s.Diagnostics) == 0}
	res.Diagnostics = s.Diagnostics

	blob, err := base64.StdEncoding.DecodeString(q.KeyB64)
	if err != nil {
		res.Verdict = VerdictInvalid
		res.Reason = fmt.Sprintf("presented key is not valid base64: %v", err)
		return res
	}
	presented, err := ValidateKey(q.KeyType, blob)
	if err != nil {
		if q.KeyType != KeyTypeEd25519 {
			res.Verdict = VerdictUnsupported
		} else {
			res.Verdict = VerdictInvalid
		}
		res.Reason = err.Error()
		return res
	}
	res.Fingerprint = presented.Fingerprint

	var trusted, changed bool
	for _, e := range s.Entries {
		if !MatchPatterns(e.Patterns, q.Target) {
			continue
		}
		ref := LineRef{Line: e.Line, Kind: kindName(e.Kind)}
		key, kerr := ValidateKey(e.KeyType, e.KeyData)
		if kerr != nil {
			// A matched line we cannot validate is a diagnostic, never a
			// credential, and makes the audit incomplete.
			res.Complete = false
			ref.Note = "unusable record: " + kerr.Error()
			res.Diagnostics = append(res.Diagnostics, Diagnostic{Line: e.Line, Reason: kerr.Error()})
			res.Lines = append(res.Lines, ref)
			continue
		}
		ref.Fingerprint = key.Fingerprint
		ref.SameKey = SameKey(key, presented)
		res.Lines = append(res.Lines, ref)
		switch e.Kind {
		case KindRevoked:
			if ref.SameKey {
				res.Verdict = VerdictRevoked
				res.Reason = fmt.Sprintf("key is revoked by line %d", e.Line)
				return res
			}
		case KindCertAuthority:
			res.Complete = false
			res.Diagnostics = append(res.Diagnostics, Diagnostic{Line: e.Line, Reason: "cert-authority entries are not supported"})
		case KindNormal:
			if ref.SameKey {
				trusted = true
			} else {
				changed = true
			}
		}
	}

	switch {
	case trusted && !res.Complete:
		res.Verdict = VerdictInconclusive
		res.Reason = "key matches a known_hosts line, but the sample contains errors; audit is incomplete"
	case trusted:
		res.Verdict = VerdictTrusted
		res.Reason = "key matches a known_hosts entry"
	case changed:
		res.Verdict = VerdictChanged
		res.Reason = "host is known but the presented key differs from all matching entries"
	default:
		res.Verdict = VerdictUnknown
		res.Reason = "no known_hosts entry matches this target (first sight)"
	}
	return res
}

func kindName(k LineKind) string {
	switch k {
	case KindRevoked:
		return "revoked"
	case KindCertAuthority:
		return "cert-authority"
	default:
		return "normal"
	}
}
