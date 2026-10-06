package spf

import (
	"fmt"
	"net/netip"
	"strings"
)

// evalMech evaluates one mechanism. It returns whether it matched;
// a non-empty res of temperror/permerror aborts the whole evaluation.
func (s *session) evalMech(domain string, m mechanism) (bool, Result, string) {
	switch m.kind {
	case "all":
		return true, "", ""
	case "ip4":
		return s.matchIPNet(m, 4)
	case "ip6":
		return s.matchIPNet(m, 6)
	case "a":
		return s.matchA(domain, m)
	case "include":
		return s.matchInclude(m)
	}
	return false, ResultPermError, "unhandled mechanism " + m.kind
}

func (s *session) matchIPNet(m mechanism, family int) (bool, Result, string) {
	if (family == 4) != s.ip.Is4() {
		return false, "", ""
	}
	addr, err := netip.ParseAddr(m.value)
	if err != nil {
		return false, ResultPermError, fmt.Sprintf("bad %s address %q", m.kind, m.value)
	}
	bits := addr.BitLen()
	plen := bits
	if family == 4 && m.hasC4 {
		plen = m.cidr4
	}
	if family == 6 {
		if m.hasC6 {
			plen = m.cidr6
		} else if m.hasC4 {
			plen = m.cidr4
		}
	}
	if plen < 0 || plen > bits {
		return false, ResultPermError, fmt.Sprintf("bad prefix length for %q", m.value)
	}
	p := netip.PrefixFrom(addr, plen)
	return p.Contains(s.ip), "", ""
}

// matchA implements the "a" mechanism: resolve A or AAAA of the target
// domain according to the sender address family and compare networks.
func (s *session) matchA(domain string, m mechanism) (bool, Result, string) {
	target := m.value
	if target == "" {
		target = domain
	}
	family := 4
	if s.ip.Is6() {
		family = 6
	}
	addrs, err := s.res.addrs(target, family)
	if err != nil {
		res, _, reason := s.mapErr(target, err)
		return false, res, reason
	}
	s.log(target, "addrs", fmt.Sprintf("family=%d %d address(es)", family, len(addrs)))
	for _, a := range addrs {
		addr, err := netip.ParseAddr(a)
		if err != nil {
			return false, ResultPermError, "bad address " + a + " at " + target
		}
		bits := addr.BitLen()
		plen := bits
		if family == 4 && m.hasC4 {
			plen = m.cidr4
		}
		if family == 6 {
			if m.hasC6 {
				plen = m.cidr6
			} else if m.hasC4 {
				plen = m.cidr4
			}
		}
		if plen < 0 || plen > bits {
			return false, ResultPermError, fmt.Sprintf("bad prefix length on a:%s", target)
		}
		if netip.PrefixFrom(addr, plen).Contains(s.ip) {
			return true, "", ""
		}
	}
	return false, "", ""
}

// matchInclude matches only when the referenced domain returns pass.
// A child fail/softfail/neutral means "no match", never a parent fail.
func (s *session) matchInclude(m mechanism) (bool, Result, string) {
	target := strings.ToLower(m.value)
	s.log(target, "include", "evaluating include:"+target)
	res, _, reason := s.evalDomain(target, false)
	switch res {
	case ResultPass:
		return true, "", ""
	case ResultTempError:
		return false, ResultTempError, reason
	case ResultPermError, ResultNone:
		if res == ResultNone {
			reason = "include target " + target + " has no SPF record"
		}
		return false, ResultPermError, reason
	default: // fail, softfail, neutral -> not a match
		return false, "", ""
	}
}
