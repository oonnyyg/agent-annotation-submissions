package spf

import (
	"errors"
	"fmt"
	"strings"
)

// ErrTemperror marks a simulated transient resolver failure.
var ErrTemperror = errors.New("temporary resolver failure")

// errBudget marks exhaustion of the DNS lookup budget.
var errBudget = errors.New("DNS lookup budget exhausted")

// errVoid marks exhaustion of the void (empty) lookup allowance.
var errVoid = errors.New("void lookup limit exceeded")

const (
	// MaxLookups is the per-evaluation DNS lookup budget (RFC 7208 4.6.4).
	MaxLookups = 10
	// MaxVoid is the per-evaluation empty-answer allowance.
	MaxVoid = 2
)

// resolver answers DNS questions from a Snapshot, enforcing the lookup
// budget and void limit. Cache hits still consume budget.
type resolver struct {
	snap    *Snapshot
	budget  int
	voids   int
	txtSeen map[string][]string
	aSeen   map[string][]string
	used    int
}

func newResolver(s *Snapshot) *resolver {
	return &resolver{
		snap:    s,
		budget:  MaxLookups,
		txtSeen: map[string][]string{},
		aSeen:   map[string][]string{},
	}
}

// charge consumes one unit of budget, even on cache hits.
func (r *resolver) charge() error {
	if r.budget <= 0 {
		return errBudget
	}
	r.budget--
	r.used++
	return nil
}

func (r *resolver) failKey(qtype, name string) (string, bool) {
	v, ok := r.snap.Errors[strings.ToLower(qtype+":"+name)]
	return v, ok
}

// txt returns the concatenated TXT records for name.
func (r *resolver) txt(name string) ([]string, error) {
	name = strings.ToLower(name)
	if err := r.charge(); err != nil {
		return nil, err
	}
	if kind, bad := r.failKey("TXT", name); bad {
		if kind == "temperror" {
			return nil, ErrTemperror
		}
		return nil, fmt.Errorf("resolver error %q for %s", kind, name)
	}
	if recs, ok := r.txtSeen[name]; ok {
		return recs, nil
	}
	zone, ok := r.snap.Zones[name]
	if !ok || len(zone.TXT) == 0 {
		r.voids++
		if r.voids > MaxVoid {
			return nil, errVoid
		}
		r.txtSeen[name] = nil
		return nil, nil
	}
	out := make([]string, 0, len(zone.TXT))
	for _, segs := range zone.TXT {
		out = append(out, strings.Join(segs, ""))
	}
	r.txtSeen[name] = out
	return out, nil
}

// addrs returns A (family 4) or AAAA (family 6) addresses for name.
func (r *resolver) addrs(name string, family int) ([]string, error) {
	name = strings.ToLower(name)
	if err := r.charge(); err != nil {
		return nil, err
	}
	qtype := "A"
	if family == 6 {
		qtype = "AAAA"
	}
	if kind, bad := r.failKey(qtype, name); bad {
		if kind == "temperror" {
			return nil, ErrTemperror
		}
		return nil, fmt.Errorf("resolver error %q for %s", kind, name)
	}
	key := fmt.Sprintf("%d:%s", family, name)
	if recs, ok := r.aSeen[key]; ok {
		return recs, nil
	}
	zone, ok := r.snap.Zones[name]
	var out []string
	if ok {
		if family == 6 {
			out = zone.AAAA
		} else {
			out = zone.A
		}
	}
	if len(out) == 0 {
		r.voids++
		if r.voids > MaxVoid {
			return nil, errVoid
		}
	}
	r.aSeen[key] = out
	return out, nil
}
