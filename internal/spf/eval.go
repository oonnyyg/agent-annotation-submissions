package spf

import (
	"fmt"
	"net"
	"strings"
)

const (
	ResultPass      = "pass"
	ResultFail      = "fail"
	ResultSoftFail  = "softfail"
	ResultNeutral   = "neutral"
	ResultNone      = "none"
	ResultTempError = "temperror"
	ResultPermError = "permerror"
)

const (
	MaxLookups     = 10
	MaxVoidLookups = 2
)

type TraceStep struct {
	Depth  int    `json:"depth"`
	Domain string `json:"domain"`
	Detail string `json:"detail"`
}

type Result struct {
	IP          string      `json:"ip"`
	Domain      string      `json:"domain"`
	Result      string      `json:"result"`
	Reason      string      `json:"reason"`
	MatchedRule string      `json:"matched_rule,omitempty"`
	BudgetUsed  int         `json:"budget_used"`
	VoidLookups int         `json:"void_lookups"`
	Trace       []TraceStep `json:"trace"`
}

type Evaluator struct {
	snap *Snapshot
}

func NewEvaluator(s *Snapshot) *Evaluator { return &Evaluator{snap: s} }

func (e *Evaluator) Check(ip, domain string) Result {
	st := &evalState{
		snap:   e.snap,
		ip:     net.ParseIP(ip),
		cache:  map[string]*Record{},
		noRec:  map[string]bool{},
		frames: map[string]bool{},
	}
	res := Result{IP: ip, Domain: domain, Trace: []TraceStep{}}
	if st.ip == nil {
		res.Result = ResultPermError
		res.Reason = fmt.Sprintf("invalid IP address %q", ip)
		return res
	}
	outcome, matched := st.eval(domain, 0, &res)
	res.Result = outcome
	res.MatchedRule = matched
	res.BudgetUsed = st.lookups
	res.VoidLookups = st.voids
	if res.Reason == "" {
		res.Reason = defaultReason(outcome, matched)
	}
	return res
}

func defaultReason(outcome, matched string) string {
	switch outcome {
	case ResultNone:
		return "no SPF record published"
	case ResultNeutral:
		return "no rule matched and no redirect applies"
	}
	if matched != "" {
		return "matched rule " + matched
	}
	return outcome
}

type evalState struct {
	snap    *Snapshot
	ip      net.IP
	lookups int
	voids   int
	cache   map[string]*Record
	noRec   map[string]bool
	frames  map[string]bool
}

func (st *evalState) trace(res *Result, depth int, domain, format string, args ...any) {
	res.Trace = append(res.Trace, TraceStep{
		Depth:  depth,
		Domain: domain,
		Detail: fmt.Sprintf(format, args...),
	})
}

// charge consumes one DNS-lookup-term of budget. Cached answers still cost.
func (st *evalState) charge() error {
	st.lookups++
	if st.lookups > MaxLookups {
		return fmt.Errorf("lookup limit of %d exceeded", MaxLookups)
	}
	return nil
}

func (st *evalState) chargeVoid() error {
	st.voids++
	if st.voids > MaxVoidLookups {
		return fmt.Errorf("void lookup limit of %d exceeded", MaxVoidLookups)
	}
	return nil
}

// recordFor returns the parsed SPF record for domain, charging budget.
// found=false with err=nil means no SPF record exists.
func (st *evalState) recordFor(domain string, res *Result, depth int) (rec *Record, found bool, err error) {
	d := strings.ToLower(domain)
	if err := st.charge(); err != nil {
		return nil, false, &permError{err.Error()}
	}
	if st.noRec[d] {
		st.trace(res, depth, d, "TXT %s: cached (no SPF record)", d)
		return nil, false, nil
	}
	if rec, ok := st.cache[d]; ok {
		st.trace(res, depth, d, "TXT %s: cached record %q", d, rec.Raw)
		return rec, true, nil
	}
	answers, lerr := st.snap.Lookup(d, "TXT")
	if lerr != nil {
		return nil, false, lerr
	}
	var spfRecords []string
	for _, segments := range answers {
		joined := strings.Join(segments, "")
		if strings.HasPrefix(joined, "v=spf1") && (len(joined) == 6 || joined[6] == ' ') {
			spfRecords = append(spfRecords, joined)
		}
	}
	switch len(spfRecords) {
	case 0:
		st.trace(res, depth, d, "TXT %s: no SPF record", d)
		st.noRec[d] = true
		if err := st.chargeVoid(); err != nil {
			return nil, false, &permError{err.Error()}
		}
		return nil, false, nil
	case 1:
		st.trace(res, depth, d, "TXT %s: %q", d, spfRecords[0])
	default:
		return nil, false, &permError{fmt.Sprintf("%s publishes %d SPF records", d, len(spfRecords))}
	}
	rec, perr := ParseRecord(strings.TrimPrefix(spfRecords[0], "v=spf1"))
	if perr != nil {
		return nil, false, &permError{fmt.Sprintf("%s: %s", d, perr.Error())}
	}
	st.cache[d] = rec
	return rec, true, nil
}

type permError struct{ msg string }

func (e *permError) Error() string { return e.msg }

// eval evaluates domain's SPF record against the sender IP.
func (st *evalState) eval(domain string, depth int, res *Result) (string, string) {
	d := strings.ToLower(domain)
	if st.frames[d] {
		res.Reason = fmt.Sprintf("include/redirect loop detected at %s", d)
		return ResultPermError, ""
	}
	st.frames[d] = true
	defer delete(st.frames, d)

	rec, found, err := st.recordFor(d, res, depth)
	if err != nil {
		return st.errorResult(err, res)
	}
	if !found {
		return ResultNone, ""
	}
	for _, m := range rec.Mechanisms {
		matched, outcome, merr := st.matchMechanism(m, d, depth, res)
		if merr != nil {
			return st.errorResult(merr, res)
		}
		if outcome != "" {
			// include sub-evaluation returned temperror/permerror.
			return outcome, m.Raw
		}
		if matched {
			return qualifierResult(m.Qual), m.Raw
		}
	}
	if rec.Redirect != "" && !rec.HasAll {
		st.trace(res, depth, d, "no mechanism matched; following redirect=%s", rec.Redirect)
		outcome, _ := st.eval(rec.Redirect, depth+1, res)
		if outcome == ResultNone {
			res.Reason = fmt.Sprintf("redirect target %s has no SPF record", rec.Redirect)
			return ResultPermError, ""
		}
		return outcome, ""
	}
	return ResultNeutral, ""
}

func (st *evalState) errorResult(err error, res *Result) (string, string) {
	if IsTemp(err) {
		res.Reason = err.Error()
		return ResultTempError, ""
	}
	if pe, ok := err.(*permError); ok {
		res.Reason = pe.msg
		return ResultPermError, ""
	}
	res.Reason = err.Error()
	return ResultPermError, ""
}

func qualifierResult(q Qualifier) string {
	switch q {
	case QFail:
		return ResultFail
	case QSoftFail:
		return ResultSoftFail
	case QNeutral:
		return ResultNeutral
	default:
		return ResultPass
	}
}

// matchMechanism reports whether m matched. A non-empty outcome means the
// include sub-evaluation produced a decisive non-pass result that must be
// surfaced directly (RFC 7208 §5.2).
func (st *evalState) matchMechanism(m Mechanism, domain string, depth int, res *Result) (bool, string, error) {
	switch m.Name {
	case "all":
		st.trace(res, depth, domain, "all matches everything")
		return true, "", nil
	case "ip4":
		return st.matchIP(m, 4, domain, depth, res)
	case "ip6":
		return st.matchIP(m, 6, domain, depth, res)
	case "a":
		return st.matchA(m, domain, depth, res)
	case "include":
		return st.matchInclude(m, domain, depth, res)
	}
	return false, "", &permError{"unsupported mechanism " + m.Name}
}

func (st *evalState) matchIP(m Mechanism, family int, domain string, depth int, res *Result) (bool, string, error) {
	ip := st.ip
	if family == 4 {
		if ip.To4() == nil {
			return false, "", nil
		}
	} else if ip.To4() != nil {
		return false, "", nil
	}
	bits := 32
	cidr := m.CIDR4
	if family == 6 {
		bits = 128
		cidr = m.CIDR6
	}
	if cidr < 0 {
		cidr = bits
	}
	if cidr > bits {
		return false, "", &permError{fmt.Sprintf("cidr /%d too large for ip%d", cidr, family)}
	}
	_, network, err := net.ParseCIDR(fmt.Sprintf("%s/%d", m.Value, cidr))
	if err != nil {
		return false, "", &permError{fmt.Sprintf("bad %s network %q: %s", m.Name, m.Value, err)}
	}
	hit := network.Contains(ip)
	st.trace(res, depth, domain, "%s %s/%d vs %s: match=%v", m.Name, m.Value, cidr, ip, hit)
	return hit, "", nil
}

func (st *evalState) matchA(m Mechanism, domain string, depth int, res *Result) (bool, string, error) {
	target := m.Value
	if target == "" {
		target = domain
	}
	target = strings.ToLower(target)
	rrtype := "A"
	if st.ip.To4() == nil {
		rrtype = "AAAA"
	}
	if err := st.charge(); err != nil {
		return false, "", &permError{err.Error()}
	}
	answers, err := st.snap.Lookup(target, rrtype)
	if err != nil {
		return false, "", err
	}
	if len(answers) == 0 {
		st.trace(res, depth, domain, "%s %s: no addresses", rrtype, target)
		if verr := st.chargeVoid(); verr != nil {
			return false, "", &permError{verr.Error()}
		}
		return false, "", nil
	}
	bits := 32
	cidr := m.CIDR4
	if rrtype == "AAAA" {
		bits = 128
		cidr = m.CIDR6
	}
	if cidr < 0 {
		cidr = bits
	}
	if cidr > bits {
		return false, "", &permError{fmt.Sprintf("cidr /%d too large for %s", cidr, rrtype)}
	}
	for _, segments := range answers {
		for _, addr := range segments {
			parsed := net.ParseIP(addr)
			if parsed == nil {
				continue
			}
			n := &net.IPNet{IP: parsed, Mask: net.CIDRMask(cidr, bits)}
			if n.Contains(st.ip) {
				st.trace(res, depth, domain, "%s %s -> %s/%d matches %s", rrtype, target, addr, cidr, st.ip)
				return true, "", nil
			}
		}
	}
	st.trace(res, depth, domain, "%s %s: no address matches %s", rrtype, target, st.ip)
	return false, "", nil
}

func (st *evalState) matchInclude(m Mechanism, domain string, depth int, res *Result) (bool, string, error) {
	st.trace(res, depth, domain, "include:%s", m.Value)
	outcome, _ := st.eval(m.Value, depth+1, res)
	switch outcome {
	case ResultPass:
		return true, "", nil
	case ResultFail, ResultSoftFail, ResultNeutral:
		// A child rejection does not match; the parent keeps evaluating.
		return false, "", nil
	case ResultNone:
		return false, "", &permError{fmt.Sprintf("include:%s has no SPF record", m.Value)}
	default: // temperror, permerror propagate
		return false, outcome, nil
	}
}
