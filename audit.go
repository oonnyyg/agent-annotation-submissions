package ntpaudit

import (
	"encoding/hex"
	"fmt"
	"math"
	"time"
)

// Record is one JSONL exchange record: a captured NTP request/response pair
// with local send/receive times and an approximate UTC base used to
// resolve the era of the 32-bit NTP seconds.
type Record struct {
	Peer          string    `json:"peer"`
	RequestHex    string    `json:"request_hex"`
	ResponseHex   string    `json:"response_hex"`
	SentLocal     time.Time `json:"sent_local"`
	ReceivedLocal time.Time `json:"received_local"`
	ApproxUTC     time.Time `json:"approx_utc"`
}

// Sample is a valid four-timestamp offset measurement.
type Sample struct {
	Offset time.Duration
	Delay  time.Duration
	T1     time.Time // client transmit (local clock)
	T2     time.Time // server receive (expanded UTC)
	T3     time.Time // server transmit (expanded UTC)
	T4     time.Time // client receive (local clock)
}

// Status classifies the outcome of analysing one record.
type Status string

const (
	StatusAccepted  Status = "accepted"
	StatusRejected  Status = "rejected"
	StatusDuplicate Status = "duplicate"
)

// Analysis is the per-record audit result.
type Analysis struct {
	Line   int
	Peer   string
	Status Status
	Reason string
	Sample *Sample
}

// peerState keeps the sliding window of valid samples for one peer.
type peerState struct {
	window   []Sample
	answered map[Timestamp]bool
}

// Auditor processes exchange records and maintains per-peer windows.
type Auditor struct {
	windowSize int
	peers      map[string]*peerState
	order      []string
}

// NewAuditor returns an Auditor keeping the windowSize most recent valid
// samples per peer.
func NewAuditor(windowSize int) *Auditor {
	if windowSize < 1 {
		windowSize = 1
	}
	return &Auditor{windowSize: windowSize, peers: map[string]*peerState{}}
}

func (a *Auditor) peer(name string) *peerState {
	p, ok := a.peers[name]
	if !ok {
		p = &peerState{answered: map[Timestamp]bool{}}
		a.peers[name] = p
		a.order = append(a.order, name)
	}
	return p
}

// Process analyses one record and updates the peer window on success.
func (a *Auditor) Process(line int, rec Record) Analysis {
	an := Analysis{Line: line, Peer: rec.Peer}
	fail := func(err error) Analysis {
		an.Status = StatusRejected
		an.Reason = err.Error()
		return an
	}
	st := a.peer(rec.Peer)

	reqBytes, err := hex.DecodeString(rec.RequestHex)
	if err != nil {
		return fail(fmt.Errorf("request hex: %w", err))
	}
	respBytes, err := hex.DecodeString(rec.ResponseHex)
	if err != nil {
		return fail(fmt.Errorf("response hex: %w", err))
	}
	req, err := ParsePacket(reqBytes)
	if err != nil {
		return fail(fmt.Errorf("request: %w", err))
	}
	resp, err := ParsePacket(respBytes)
	if err != nil {
		return fail(fmt.Errorf("response: %w", err))
	}
	if err := req.ValidateRequest(); err != nil {
		return fail(err)
	}
	if err := resp.ValidateResponse(req); err != nil {
		return fail(err)
	}
	if st.answered[req.Transmit] {
		an.Status = StatusDuplicate
		an.Reason = "response duplicates an already answered request"
		return an
	}
	t2, err := resp.Receive.Expand(rec.ApproxUTC)
	if err != nil {
		return fail(fmt.Errorf("receive timestamp: %w", err))
	}
	t3, err := resp.Transmit.Expand(rec.ApproxUTC)
	if err != nil {
		return fail(fmt.Errorf("transmit timestamp: %w", err))
	}
	if t3.Before(t2) {
		return fail(fmt.Errorf("server transmit %s precedes server receive %s", t3.Format(time.RFC3339Nano), t2.Format(time.RFC3339Nano)))
	}
	t1, t4 := rec.SentLocal, rec.ReceivedLocal
	delay := t4.Sub(t1) - t3.Sub(t2)
	if delay < 0 {
		return fail(fmt.Errorf("negative round-trip delay %s", delay))
	}
	offset := (t2.Sub(t1) + t3.Sub(t4)) / 2
	s := Sample{Offset: offset, Delay: delay, T1: t1, T2: t2, T3: t3, T4: t4}
	st.answered[req.Transmit] = true
	st.window = append(st.window, s)
	if len(st.window) > a.windowSize {
		st.window = st.window[len(st.window)-a.windowSize:]
	}
	an.Status = StatusAccepted
	an.Sample = &s
	return an
}

// PeerSummary is the per-peer selection result over the current window.
type PeerSummary struct {
	Peer     string
	Window   []Sample
	Selected *Sample
	Jitter   time.Duration
}

// Summaries returns a summary for every peer seen, in first-seen order.
// The selected sample is the window sample with the smallest delay; ties
// resolve to the most recent one. Jitter is the RMS of the other window
// samples' offsets relative to the selected offset; a single-sample window
// has zero jitter.
func (a *Auditor) Summaries() []PeerSummary {
	out := make([]PeerSummary, 0, len(a.order))
	for _, name := range a.order {
		st := a.peers[name]
		sum := PeerSummary{Peer: name, Window: append([]Sample(nil), st.window...)}
		if len(st.window) == 0 {
			out = append(out, sum)
			continue
		}
		sel := 0
		for i := 1; i < len(st.window); i++ {
			if st.window[i].Delay <= st.window[sel].Delay {
				sel = i
			}
		}
		s := st.window[sel]
		sum.Selected = &s
		if len(st.window) > 1 {
			var acc float64
			for i, w := range st.window {
				if i == sel {
					continue
				}
				d := float64(w.Offset - s.Offset)
				acc += d * d
			}
			sum.Jitter = time.Duration(math.Sqrt(acc / float64(len(st.window)-1)))
		}
		out = append(out, sum)
	}
	return out
}
