package spf

import (
	"fmt"
	"strconv"
	"strings"
)

type Qualifier rune

const (
	QPass     Qualifier = '+'
	QFail     Qualifier = '-'
	QSoftFail Qualifier = '~'
	QNeutral  Qualifier = '?'
)

type Mechanism struct {
	Qual  Qualifier
	Name  string // ip4, ip6, a, include, all
	Value string // network/address or domain for include
	CIDR4 int    // -1 when absent
	CIDR6 int
	Raw   string
}

type Modifier struct {
	Name  string // redirect
	Value string
}

type Record struct {
	Raw        string
	Mechanisms []Mechanism
	Redirect   string
	HasAll     bool
}

var errUnsupported = func(tok string) error {
	return fmt.Errorf("unsupported mechanism or modifier %q: only ip4, ip6, a, include, all and redirect are in scope", tok)
}

// ParseRecord parses an SPF record body (after "v=spf1").
func ParseRecord(raw string) (*Record, error) {
	r := &Record{Raw: raw}
	for _, tok := range strings.Fields(raw) {
		if strings.Contains(tok, "%{") {
			return nil, fmt.Errorf("macros are out of scope: %q", tok)
		}
		if name, val, ok := splitModifier(tok); ok {
			if name != "redirect" {
				return nil, errUnsupported(tok)
			}
			if r.Redirect != "" {
				return nil, fmt.Errorf("duplicate redirect modifier")
			}
			r.Redirect = val
			continue
		}
		m, err := parseMechanism(tok)
		if err != nil {
			return nil, err
		}
		if m.Name == "all" {
			r.HasAll = true
		}
		r.Mechanisms = append(r.Mechanisms, m)
	}
	return r, nil
}

func splitModifier(tok string) (name, value string, ok bool) {
	i := strings.IndexByte(tok, '=')
	if i < 0 {
		return "", "", false
	}
	return strings.ToLower(tok[:i]), tok[i+1:], true
}

func parseMechanism(tok string) (Mechanism, error) {
	m := Mechanism{Qual: QPass, CIDR4: -1, CIDR6: -1, Raw: tok}
	s := tok
	if c := s[0]; c == '+' || c == '-' || c == '~' || c == '?' {
		m.Qual = Qualifier(c)
		s = s[1:]
	}
	name, rest := s, ""
	if i := strings.IndexAny(s, ":/"); i >= 0 {
		name, rest = s[:i], s[i:]
	}
	m.Name = strings.ToLower(name)
	switch m.Name {
	case "all":
		if rest != "" {
			return m, fmt.Errorf("all takes no arguments: %q", tok)
		}
	case "include":
		if !strings.HasPrefix(rest, ":") || rest == ":" {
			return m, fmt.Errorf("include requires a domain: %q", tok)
		}
		m.Value = rest[1:]
	case "a":
		if strings.HasPrefix(rest, ":") {
			j := strings.IndexByte(rest, '/')
			if j < 0 {
				m.Value = rest[1:]
				rest = ""
			} else {
				m.Value = rest[1:j]
				rest = rest[j:]
			}
		}
		if err := parseCIDRs(rest, &m); err != nil {
			return m, fmt.Errorf("%q: %w", tok, err)
		}
	case "ip4", "ip6":
		if !strings.HasPrefix(rest, ":") {
			return m, fmt.Errorf("%s requires an address: %q", m.Name, tok)
		}
		body := rest[1:]
		if j := strings.IndexByte(body, '/'); j >= 0 {
			m.Value = body[:j]
			cidr, err := strconv.Atoi(body[j+1:])
			if err != nil {
				return m, fmt.Errorf("bad cidr in %q", tok)
			}
			if m.Name == "ip4" {
				m.CIDR4 = cidr
			} else {
				m.CIDR6 = cidr
			}
		} else {
			m.Value = body
		}
	default:
		return m, errUnsupported(tok)
	}
	return m, nil
}

func parseCIDRs(rest string, m *Mechanism) error {
	for rest != "" {
		if !strings.HasPrefix(rest, "/") {
			return fmt.Errorf("unexpected trailing data")
		}
		body := rest[1:]
		dual := strings.HasPrefix(body, "/")
		if dual {
			body = body[1:]
		}
		j := strings.IndexByte(body, '/')
		num := body
		if j >= 0 {
			num = body[:j]
		}
		v, err := strconv.Atoi(num)
		if err != nil {
			return fmt.Errorf("bad cidr length")
		}
		if dual {
			m.CIDR6 = v
		} else {
			m.CIDR4 = v
		}
		if j < 0 {
			return nil
		}
		rest = body[j:]
	}
	return nil
}
