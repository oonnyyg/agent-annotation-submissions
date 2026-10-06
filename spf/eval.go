package spf

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
)

// Result is the SPF verdict for one query.
type Result string

const (
	ResultPass      Result = "pass"
	ResultFail      Result = "fail"
	ResultSoftFail  Result = "softfail"
	ResultNeutral   Result = "neutral"
	ResultNone      Result = "none"
	ResultPermError Result = "permerror"
	ResultTempError Result = "temperror"
)

// Step is one piece of evidence in the evaluation chain.
type Step struct {
	Domain string `json:"domain"`
	Action string `json:"action"`
	Detail string `json:"detail"`
}

// Evaluation is the top-level outcome of one check.
type Evaluation struct {
	IP       string `json:"ip"`
	Domain   string `json:"domain"`
	Result   Result `json:"result"`
	Matched  string `json:"matched_rule,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Lookups  int    `json:"lookups_used"`
	Evidence []Step `json:"evidence"`
}

// Evaluator evaluates SPF checks against a Snapshot.
type Evaluator struct {
	snap *Snapshot
}

// NewEvaluator builds an Evaluator from a snapshot.
func NewEvaluator(s *Snapshot) *Evaluator { return &Evaluator{snap: s} }

// Check evaluates the SPF policy of domain for sender ip.
// Each call gets a fresh lookup budget; failures do not affect others.
func (e *Evaluator) Check(ip, domain string) Evaluation {
	ev := Evaluation{IP: ip, Domain: domain}
	parsed, err := netip.ParseAddr(ip)
	if err != nil {
		ev.Result = ResultPermError
		ev.Reason = "invalid sender IP: " + ip
		return ev
	}
	sess := &session{
		res:      newResolver(e.snap),
		ip:       parsed,
		visiting: map[string]bool{},
	}
	res, matched, reason := sess.evalDomain(strings.ToLower(domain), true)
	ev.Result = res
	ev.Matched = matched
	ev.Reason = reason
	ev.Lookups = sess.res.used
	ev.Evidence = sess.steps
	return ev
}

// session carries per-check state: budget, evidence, recursion guard.
type session struct {
	res      *resolver
	ip       netip.Addr
	steps    []Step
	visiting map[string]bool
}

func (s *session) log(domain, action, detail string) {
	s.steps = append(s.steps, Step{Domain: domain, Action: action, Detail: detail})
}

// evalDomain evaluates the SPF record of domain. top marks the
// top-level query (affects none-result reporting).
func (s *session) evalDomain(domain string, top bool) (Result, string, string) {
	if s.visiting[domain] {
		s.log(domain, "error", "include/redirect loop detected")
		return ResultPermError, "", "loop involving " + domain
	}
	s.visiting[domain] = true
	defer delete(s.visiting, domain)

	txts, err := s.res.txt(domain)
	if err != nil {
		return s.mapErr(domain, err)
	}
	s.log(domain, "txt", fmt.Sprintf("%d TXT record(s)", len(txts)))
	body, err := selectSPF(txts)
	if err != nil {
		s.log(domain, "error", err.Error())
		return ResultPermError, "", err.Error()
	}
	if body == "" {
		s.log(domain, "none", "no SPF record")
		if top {
			return ResultNone, "", "no SPF record for " + domain
		}
		return ResultNone, "", "no SPF record for " + domain
	}
	rec, err := parseRecord(body)
	if err != nil {
		s.log(domain, "error", err.Error())
		return ResultPermError, "", err.Error()
	}
	s.log(domain, "record", "v=spf1"+body)

	for _, m := range rec.mechs {
		match, res, reason := s.evalMech(domain, m)
		if res == ResultTempError || res == ResultPermError {
			return res, "", reason
		}
		if match {
			r := m.qual.result()
			s.log(domain, "match", fmt.Sprintf("mechanism %q matched -> %s", mechString(m), r))
			return r, mechString(m), ""
		}
		s.log(domain, "nomatch", fmt.Sprintf("mechanism %q did not match", mechString(m)))
	}

	if rec.redirect != "" && !rec.hasAll {
		target := strings.ToLower(rec.redirect)
		s.log(domain, "redirect", "no rule matched; following redirect="+target)
		res, matched, reason := s.evalDomain(target, false)
		if res == ResultNone {
			s.log(domain, "error", "redirect target has no SPF record")
			return ResultPermError, "", "redirect target " + target + " has no SPF record"
		}
		return res, matched, reason
	}
	s.log(domain, "neutral", "no rule matched and no redirect")
	return ResultNeutral, "", ""
}

func (s *session) mapErr(domain string, err error) (Result, string, string) {
	switch {
	case errors.Is(err, ErrTemperror):
		s.log(domain, "error", "temporary resolver failure")
		return ResultTempError, "", "temporary resolver failure at " + domain
	case errors.Is(err, errBudget):
		s.log(domain, "error", "DNS lookup budget exhausted")
		return ResultPermError, "", "DNS lookup limit exceeded at " + domain
	case errors.Is(err, errVoid):
		s.log(domain, "error", "void lookup limit exceeded")
		return ResultPermError, "", "void lookup limit exceeded at " + domain
	default:
		s.log(domain, "error", err.Error())
		return ResultPermError, "", err.Error()
	}
}

func mechString(m mechanism) string {
	var b strings.Builder
	if m.qual != qPass {
		b.WriteByte(byte(m.qual))
	}
	b.WriteString(m.kind)
	if m.value != "" {
		b.WriteString(":")
		b.WriteString(m.value)
	}
	if m.hasC4 {
		fmt.Fprintf(&b, "/%d", m.cidr4)
	}
	if m.hasC6 {
		fmt.Fprintf(&b, "//%d", m.cidr6)
	}
	return b.String()
}
