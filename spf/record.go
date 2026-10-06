package spf

import (
	"fmt"
	"strconv"
	"strings"
)

// qualifier is the result modifier of a mechanism.
type qualifier byte

const (
	qPass     qualifier = '+'
	qFail     qualifier = '-'
	qSoftFail qualifier = '~'
	qNeutral  qualifier = '?'
)

func (q qualifier) result() Result {
	switch q {
	case qFail:
		return ResultFail
	case qSoftFail:
		return ResultSoftFail
	case qNeutral:
		return ResultNeutral
	default:
		return ResultPass
	}
}

// mechanism is one parsed SPF term.
type mechanism struct {
	qual  qualifier
	kind  string // "ip4", "ip6", "a", "include", "all"
	value string // network or domain argument
	cidr4 int    // v4 prefix length, -1 = default
	cidr6 int    // v6 prefix length, -1 = default
	hasC4 bool
	hasC6 bool
}

// record is a parsed SPF record.
type record struct {
	mechs    []mechanism
	redirect string // redirect= target, "" if absent
	hasAll   bool
}

// unsupportedKinds are RFC 7208 mechanisms deliberately out of scope.
var unsupportedKinds = map[string]bool{
	"mx": true, "ptr": true, "exists": true,
}

// parseRecord parses an SPF record body (after "v=spf1").
// Macros and unsupported mechanisms are rejected as out of scope.
func parseRecord(body string) (*record, error) {
	rec := &record{}
	for _, term := range strings.Fields(body) {
		if strings.Contains(term, "%") {
			return nil, fmt.Errorf("macro %q is out of scope", term)
		}
		if strings.HasPrefix(term, "redirect=") {
			rec.redirect = strings.TrimPrefix(term, "redirect=")
			continue
		}
		if strings.Contains(term, "=") {
			return nil, fmt.Errorf("unknown modifier %q is out of scope", term)
		}
		m, err := parseMechanism(term)
		if err != nil {
			return nil, err
		}
		if m.kind == "all" {
			rec.hasAll = true
		}
		rec.mechs = append(rec.mechs, m)
	}
	return rec, nil
}

func parseMechanism(term string) (mechanism, error) {
	m := mechanism{qual: qPass, cidr4: -1, cidr6: -1}
	switch term[0] {
	case '+', '-', '~', '?':
		m.qual = qualifier(term[0])
		term = term[1:]
	}
	name, arg, found := strings.Cut(term, ":")
	if !found {
		// "a/24//64" and "all" carry no ":"; split cidr off the name.
		if i := strings.IndexByte(name, '/'); i >= 0 {
			name, arg, found = name[:i], name[i:], true
		}
	}
	if name == "all" || name == "include" || name == "a" ||
		name == "ip4" || name == "ip6" {
		// supported set
	} else if unsupportedKinds[name] {
		return m, fmt.Errorf("mechanism %q is out of scope", name)
	} else {
		return m, fmt.Errorf("unknown mechanism %q", name)
	}
	m.kind = name
	// Dual cidr: value may carry "/ip4len//ip6len".
	if i := strings.Index(arg, "//"); i >= 0 {
		m.cidr6, _ = strconv.Atoi(arg[i+2:])
		m.hasC6 = true
		arg = arg[:i]
	}
	if i := strings.IndexByte(arg, '/'); i >= 0 {
		m.cidr4, _ = strconv.Atoi(arg[i+1:])
		m.hasC4 = true
		arg = arg[:i]
	}
	m.value = arg
	switch name {
	case "all":
		if arg != "" {
			return m, fmt.Errorf("all takes no argument")
		}
	case "include":
		if arg == "" {
			return m, fmt.Errorf("include requires a domain")
		}
	case "ip4", "ip6":
		if arg == "" {
			return m, fmt.Errorf("%s requires an address", name)
		}
	}
	return m, nil
}

// selectSPF picks the single SPF record from TXT answers.
// Returns ("", nil) when none exists; errors when several exist.
func selectSPF(txts []string) (string, error) {
	var found []string
	for _, t := range txts {
		if t == "v=spf1" || strings.HasPrefix(t, "v=spf1 ") {
			found = append(found, t)
		}
	}
	switch len(found) {
	case 0:
		return "", nil
	case 1:
		return strings.TrimPrefix(found[0], "v=spf1"), nil
	default:
		return "", fmt.Errorf("multiple SPF records (%d)", len(found))
	}
}
