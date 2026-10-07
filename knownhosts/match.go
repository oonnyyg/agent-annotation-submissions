package knownhosts

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// DefaultPort is the SSH port implied by bare hostnames.
const DefaultPort = 22

// Target identifies a host:port pair to audit. Bare hostnames and
// "[host]:port" forms are distinct targets and never conflated.
type Target struct {
	Host string
	Port int
}

func (t Target) String() string {
	if t.Port == DefaultPort {
		return t.Host
	}
	return fmt.Sprintf("[%s]:%d", t.Host, t.Port)
}

// ParseTarget parses "host", "host:port" or "[host]:port" query forms.
func ParseTarget(s string) (Target, error) {
	if s == "" {
		return Target{}, fmt.Errorf("empty target")
	}
	if strings.HasPrefix(s, "[") {
		end := strings.Index(s, "]")
		if end < 0 || end+1 >= len(s) || s[end+1] != ':' {
			return Target{}, fmt.Errorf("invalid bracketed target %q, want [host]:port", s)
		}
		port, err := strconv.Atoi(s[end+2:])
		if err != nil || port < 1 || port > 65535 {
			return Target{}, fmt.Errorf("invalid port in %q", s)
		}
		return Target{Host: s[1:end], Port: port}, nil
	}
	if strings.Contains(s, ":") {
		h, p, err := splitHostPort(s)
		if err != nil {
			return Target{}, fmt.Errorf("invalid target %q: %v", s, err)
		}
		return Target{Host: h, Port: p}, nil
	}
	return Target{Host: s, Port: DefaultPort}, nil
}

func splitHostPort(s string) (string, int, error) {
	i := strings.LastIndex(s, ":")
	if i < 0 {
		return "", 0, fmt.Errorf("no port")
	}
	port, err := strconv.Atoi(s[i+1:])
	if err != nil || port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("bad port")
	}
	return s[:i], port, nil
}

// MatchPatterns reports whether any pattern list of the entry matches the
// target. A leading '!' negates only that pattern within its own line.
func MatchPatterns(patterns []string, t Target) bool {
	// A negated pattern that matches excludes the whole line, regardless
	// of order; it never affects other lines.
	for _, p := range patterns {
		if strings.HasPrefix(p, "!") && matchOne(p[1:], t) {
			return false
		}
	}
	for _, p := range patterns {
		if !strings.HasPrefix(p, "!") && matchOne(p, t) {
			return true
		}
	}
	return false
}

func matchOne(pattern string, t Target) bool {
	if strings.HasPrefix(pattern, "|1|") {
		return matchHashed(pattern, t)
	}
	return wildcardMatch(pattern, patternForm(t), true)
}

// patternForm renders the target the way known_hosts lists it: bare host
// for the default port, "[host]:port" otherwise.
func patternForm(t Target) string {
	return t.String()
}

// matchHashed checks an OpenSSH HASH_MAGIC "|1|salt|hash" pattern using
// HMAC-SHA1 over both the bare host and the [host]:port form, so a hashed
// entry only matches the exact target form it was generated for.
func matchHashed(pattern string, t Target) bool {
	parts := strings.SplitN(pattern[3:], "|", 2)
	if len(parts) != 2 {
		return false
	}
	salt, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	want, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	mac := hmac.New(sha1.New, salt)
	mac.Write([]byte(patternForm(t)))
	return hmac.Equal(mac.Sum(nil), want)
}

// wildcardMatch implements OpenSSH glob matching with '*' and '?'.
// Matching is case-insensitive, matching ssh's match_hostname behavior.
func wildcardMatch(pattern, s string, ci bool) bool {
	if ci {
		pattern = strings.ToLower(pattern)
		s = strings.ToLower(s)
	}
	for len(pattern) > 0 {
		switch pattern[0] {
		case '*':
			for len(pattern) > 1 && pattern[1] == '*' {
				pattern = pattern[1:]
			}
			if len(pattern) == 1 {
				return true
			}
			for i := 0; i <= len(s); i++ {
				if wildcardMatch(pattern[1:], s[i:], false) {
					return true
				}
			}
			return false
		case '?':
			if len(s) == 0 {
				return false
			}
			pattern, s = pattern[1:], s[1:]
		default:
			if len(s) == 0 || s[0] != pattern[0] {
				return false
			}
			pattern, s = pattern[1:], s[1:]
		}
	}
	return len(s) == 0
}
