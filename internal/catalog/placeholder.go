package catalog

import (
	"fmt"
	"sort"
	"strings"
)

// placeholders extracts {identifier} placeholders with occurrence counts.
// "{{" and "}}" are literal braces. Format suffixes are not supported.
func placeholders(s string) map[string]int {
	m := map[string]int{}
	for i := 0; i < len(s); {
		c := s[i]
		if c == '{' {
			if i+1 < len(s) && s[i+1] == '{' {
				i += 2
				continue
			}
			end := strings.IndexByte(s[i+1:], '}')
			if end < 0 {
				i++
				continue
			}
			name := s[i+1 : i+1+end]
			if validIdent(name) {
				m[name]++
			}
			i += end + 2
			continue
		}
		if c == '}' && i+1 < len(s) && s[i+1] == '}' {
			i += 2
			continue
		}
		i++
	}
	return m
}

func validIdent(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

func comparePlaceholders(src, dst map[string]int) string {
	var problems []string
	keys := map[string]bool{}
	for k := range src {
		keys[k] = true
	}
	for k := range dst {
		keys[k] = true
	}
	var sorted []string
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, k := range sorted {
		s, d := src[k], dst[k]
		if s != d {
			problems = append(problems, fmt.Sprintf("placeholder {%s}: source %d, translation %d", k, s, d))
		}
	}
	return strings.Join(problems, "; ")
}