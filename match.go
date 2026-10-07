package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"strings"
)

// Target is a normalized query: host plus port, default 22.
type Target struct {
	Host string
	Port int
}

// hostID returns the string matched against known_hosts patterns:
// plain hostname for the default port, [host]:port otherwise.
func (t Target) hostID() string {
	if t.Port == 22 {
		return t.Host
	}
	return "[" + t.Host + "]:" + itoa(t.Port)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// hostsFieldMatches reports whether a known_hosts hosts field matches
// the target. Comma-separated patterns, leading '!' negates (excluding
// only this line), '*' and '?' are wildcards. |1|salt|hash is an
// OpenSSH hashed hostname (HMAC-SHA1).
func hostsFieldMatches(field string, t Target) bool {
	id := t.hostID()
	matched := false
	for _, pat := range strings.Split(field, ",") {
		neg := strings.HasPrefix(pat, "!")
		if neg {
			pat = pat[1:]
		}
		if patternMatches(pat, id) {
			if neg {
				return false
			}
			matched = true
		}
	}
	return matched
}

func patternMatches(pat, id string) bool {
	if strings.HasPrefix(pat, "|1|") {
		return hashedHostMatches(pat, id)
	}
	if strings.HasPrefix(pat, "|") {
		return false // unknown hash version
	}
	return wildcardMatch(strings.ToLower(pat), strings.ToLower(id))
}

// hashedHostMatches verifies |1|base64(salt)|base64(HMAC-SHA1(salt, id)).
func hashedHostMatches(pat, id string) bool {
	parts := strings.Split(pat, "|")
	// leading empty string, "1", salt, hash
	if len(parts) != 4 || parts[1] != "1" {
		return false
	}
	salt, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	mac := hmac.New(sha1.New, salt)
	mac.Write([]byte(id))
	return hmac.Equal(mac.Sum(nil), want)
}

// wildcardMatch implements '*' and '?' glob matching.
func wildcardMatch(pat, s string) bool {
	px, sx := 0, 0
	star, mark := -1, 0
	for sx < len(s) {
		if px < len(pat) && (pat[px] == '?' || pat[px] == s[sx]) {
			px++
			sx++
		} else if px < len(pat) && pat[px] == '*' {
			star, mark = px, sx
			px++
		} else if star != -1 {
			px = star + 1
			mark++
			sx = mark
		} else {
			return false
		}
	}
	for px < len(pat) && pat[px] == '*' {
		px++
	}
	return px == len(pat)
}
