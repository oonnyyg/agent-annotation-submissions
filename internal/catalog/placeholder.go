package catalog

import "strings"

// Placeholders extracts {identifier} placeholders with occurrence counts.
// "{{" and "}}" are literal braces; format suffixes are not supported.
func Placeholders(s string) map[string]int {
	counts := map[string]int{}
	for i := 0; i < len(s); {
		c := s[i]
		if c == '{' {
			if i+1 < len(s) && s[i+1] == '{' {
				i += 2
				continue
			}
			j := i + 1
			for j < len(s) && isIdentChar(s[j], j == i+1) {
				j++
			}
			if j > i+1 && j < len(s) && s[j] == '}' {
				counts[s[i+1:j]]++
				i = j + 1
				continue
			}
			i++
			continue
		}
		if c == '}' && i+1 < len(s) && s[i+1] == '}' {
			i += 2
			continue
		}
		i++
	}
	return counts
}

func isIdentChar(b byte, first bool) bool {
	if b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') {
		return true
	}
	return !first && b >= '0' && b <= '9'
}

// EqualCounts reports whether two placeholder multisets match.
func EqualCounts(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func sortedKeys(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return strings.Join(keys, ",")
}
