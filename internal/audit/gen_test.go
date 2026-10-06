package audit

import (
	"fmt"
	"os"
	"testing"
	"time"

	"example.com/ntp-exchange-auditor/internal/ntp"
)

// TestGenSample regenerates testdata/sample.jsonl when run explicitly:
//
//	go test ./internal/audit -run TestGenSample -gen-sample
func TestGenSample(t *testing.T) {
	if os.Getenv("GEN_SAMPLE") == "" {
		t.Skip("set GEN_SAMPLE=1 to regenerate testdata/sample.jsonl")
	}
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	var out []byte
	add := func(peer string, sent time.Time, off, rtt time.Duration) {
		xmit := ts(sent)
		t2 := ts(sent.Add(rtt/2 + off))
		rec := Record{
			Peer:     peer,
			Request:  pkt(0, 4, 3, 0, "", ntp.Timestamp{}, ntp.Timestamp{}, xmit),
			Response: pkt(0, 4, 4, 2, "", xmit, t2, t2),
			SentAt:   sent.Format(time.RFC3339Nano),
			RecvAt:   sent.Add(rtt).Format(time.RFC3339Nano),
		}
		out = append(out, []byte(fmt.Sprintf(`{"peer":%q,"request":%q,"response":%q,"sent_at":%q,"received_at":%q}`+"\n",
			rec.Peer, rec.Request, rec.Response, rec.SentAt, rec.RecvAt))...)
	}
	add("ntp1.example", base, 42*time.Millisecond, 80*time.Millisecond)
	add("ntp1.example", base.Add(time.Minute), 45*time.Millisecond, 120*time.Millisecond)
	add("ntp1.example", base.Add(2*time.Minute), 41*time.Millisecond, 90*time.Millisecond)
	// Duplicate of the first request.
	out = append(out, []byte(fmt.Sprintf(`{"peer":%q,"request":%q,"response":%q,"sent_at":%q,"received_at":%q}`+"\n",
		"ntp1.example",
		pkt(0, 4, 3, 0, "", ntp.Timestamp{}, ntp.Timestamp{}, ts(base)),
		pkt(0, 4, 4, 2, "", ts(base), ts(base.Add(82*time.Millisecond)), ts(base.Add(82*time.Millisecond))),
		base.Format(time.RFC3339Nano), base.Add(100*time.Millisecond).Format(time.RFC3339Nano)))...)
	// Kiss-of-death from another peer.
	sent := base.Add(3 * time.Minute)
	out = append(out, []byte(fmt.Sprintf(`{"peer":%q,"request":%q,"response":%q,"sent_at":%q,"received_at":%q}`+"\n",
		"ntp2.example",
		pkt(0, 4, 3, 0, "", ntp.Timestamp{}, ntp.Timestamp{}, ts(sent)),
		pkt(0, 4, 4, 0, "RATE", ts(sent), ts(sent), ts(sent)),
		sent.Format(time.RFC3339Nano), sent.Add(60*time.Millisecond).Format(time.RFC3339Nano)))...)
	if err := os.WriteFile("../../testdata/sample.jsonl", out, 0o644); err != nil {
		t.Fatal(err)
	}
}
