// Package audit reconstructs clock offsets from recorded NTP exchanges.
// It never sends packets and never touches the system clock.
package audit

import (
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"time"

	"example.com/ntp-exchange-auditor/internal/ntp"
)

// Record is one JSONL input line: a single request/response exchange.
type Record struct {
	Peer     string `json:"peer"`
	Request  string `json:"request"`  // hex-encoded 48-byte client request
	Response string `json:"response"` // hex-encoded 48-byte server response
	SentAt   string `json:"sent_at"`  // local time the request was sent (RFC3339)
	RecvAt   string `json:"received_at"`
}

// Sample is a validated exchange admitted to a peer window.
type Sample struct {
	Offset time.Duration
	Delay  time.Duration
	RecvAt time.Time
	Line   int
}

// Result is the per-record analysis outcome.
type Result struct {
	Line   int
	Peer   string
	OK     bool
	Kind   string // rejection category when !OK
	Detail string
	Sample *Sample
}

func fail(line int, rec Record, kind, detail string) Result {
	return Result{Line: line, Peer: rec.Peer, Kind: kind, Detail: detail}
}

// hexToPacket decodes and parses a hex packet field.
func hexToPacket(s string) (*ntp.Packet, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return nil, fmt.Errorf("invalid hex: %v", err)
	}
	return ntp.Parse(raw)
}

// toTimestamp converts a local UTC time to an NTP timestamp.
func toTimestamp(t time.Time) ntp.Timestamp {
	sec := t.Unix() + 2208988800
	frac := uint64(t.Nanosecond()) << 32 / 1_000_000_000
	return ntp.Timestamp{Sec: sec, Frac: uint32(frac)}
}

// Analyze validates one record and, on success, returns its sample.
// It does not mutate any peer state; see Peer.Add.
func Analyze(line int, rec Record) Result {
	if rec.Peer == "" {
		return fail(line, rec, "bad-record", "missing peer identifier")
	}
	sentAt, err := time.Parse(time.RFC3339Nano, rec.SentAt)
	if err != nil {
		return fail(line, rec, "bad-record", fmt.Sprintf("bad sent_at: %v", err))
	}
	recvAt, err := time.Parse(time.RFC3339Nano, rec.RecvAt)
	if err != nil {
		return fail(line, rec, "bad-record", fmt.Sprintf("bad received_at: %v", err))
	}
	req, err := hexToPacket(rec.Request)
	if err != nil {
		return fail(line, rec, "bad-request", err.Error())
	}
	if r := ntp.ValidateRequest(req); r != nil {
		return fail(line, rec, r.Kind, r.Detail)
	}
	resp, err := hexToPacket(rec.Response)
	if err != nil {
		return fail(line, rec, "bad-response", err.Error())
	}
	if r := ntp.ValidateResponse(resp, req); r != nil {
		detail := r.Detail
		if r.KissCode != "" {
			detail = fmt.Sprintf("%s (kiss code %q)", detail, r.KissCode)
		}
		return fail(line, rec, r.Kind, detail)
	}
	// Expand 32-bit era timestamps against the local send time as the
	// approximate UTC base.
	base := sentAt
	t2, err := ntp.Expand(resp.Receive, base)
	if err != nil {
		return fail(line, rec, "era-ambiguous", "receive timestamp: "+err.Error())
	}
	t3, err := ntp.Expand(resp.Transmit, base)
	if err != nil {
		return fail(line, rec, "era-ambiguous", "transmit timestamp: "+err.Error())
	}
	if ntp.Sub(t3, t2) < 0 {
		return fail(line, rec, "invalid-sample", "server transmit precedes server receive")
	}
	t1 := toTimestamp(sentAt)
	t4 := toTimestamp(recvAt)
	offset, delay := ntp.OffsetDelay(t1, t2, t3, t4)
	if delay < 0 {
		return fail(line, rec, "invalid-sample", fmt.Sprintf("negative round-trip delay %v", delay))
	}
	return Result{
		Line: line, Peer: rec.Peer, OK: true,
		Sample: &Sample{Offset: offset, Delay: delay, RecvAt: recvAt, Line: line},
	}
}

// RequestKey extracts the request transmit timestamp, used to detect
// duplicate responses to the same request.
func RequestKey(rec Record) (ntp.Timestamp, bool) {
	req, err := hexToPacket(rec.Request)
	if err != nil {
		return ntp.Timestamp{}, false
	}
	return req.Transmit, true
}

// Peer accumulates the most recent valid samples for one peer.
type Peer struct {
	Name    string
	Window  int
	samples []Sample
	seen    map[ntp.Timestamp]bool
}

func NewPeer(name string, window int) *Peer {
	return &Peer{Name: name, Window: window, seen: map[ntp.Timestamp]bool{}}
}

// Add records the outcome of one analyzed record. It returns false when
// the record is a duplicate response to an already-answered request;
// duplicates and errors never evict valid samples.
func (p *Peer) Add(rec Record, res Result) (dup bool) {
	key, ok := RequestKey(rec)
	if ok && p.seen[key] {
		return true
	}
	if !res.OK {
		return false
	}
	if ok {
		p.seen[key] = true
	}
	p.samples = append(p.samples, *res.Sample)
	if len(p.samples) > p.Window {
		p.samples = p.samples[len(p.samples)-p.Window:]
	}
	return false
}

// Best returns the selected sample: smallest delay, latest on ties.
func (p *Peer) Best() (Sample, bool) {
	if len(p.samples) == 0 {
		return Sample{}, false
	}
	best := p.samples[0]
	for _, s := range p.samples[1:] {
		if s.Delay < best.Delay || (s.Delay == best.Delay && s.RecvAt.After(best.RecvAt)) {
			best = s
		}
	}
	return best, true
}

// Jitter is the RMS of the offsets of the other window samples relative
// to the selected offset. A single-sample window has zero jitter.
func (p *Peer) Jitter() time.Duration {
	best, ok := p.Best()
	if !ok || len(p.samples) < 2 {
		return 0
	}
	var sum float64
	var n int
	for _, s := range p.samples {
		if s.Line == best.Line {
			continue
		}
		d := float64(s.Offset - best.Offset)
		sum += d * d
		n++
	}
	return time.Duration(math.Sqrt(sum / float64(n)))
}

// Samples returns a copy of the current window.
func (p *Peer) Samples() []Sample {
	return append([]Sample(nil), p.samples...)
}
