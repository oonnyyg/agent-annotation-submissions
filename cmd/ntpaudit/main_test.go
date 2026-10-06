package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"
)

func pkt(leap, ver, mode, stratum uint8, refID string, origin, recv, xmit [8]byte) string {
	b := make([]byte, 48)
	b[0] = leap<<6 | ver<<3 | mode
	b[1] = stratum
	copy(b[12:16], refID)
	copy(b[24:32], origin[:])
	copy(b[32:40], recv[:])
	copy(b[40:48], xmit[:])
	return hex.EncodeToString(b)
}

func stamp(t time.Time) [8]byte {
	var b [8]byte
	binary.BigEndian.PutUint32(b[0:4], uint32(t.Unix()+2208988800))
	binary.BigEndian.PutUint32(b[4:8], uint32(uint64(t.Nanosecond())<<32/1_000_000_000))
	return b
}

func TestRunEndToEnd(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var lines []string
	// One valid exchange: server 40ms ahead, 100ms RTT.
	sent := base
	xmit := stamp(sent)
	t2 := stamp(sent.Add(90 * time.Millisecond))
	req := pkt(0, 4, 3, 0, "", [8]byte{}, [8]byte{}, xmit)
	resp := pkt(0, 4, 4, 2, "", xmit, t2, t2)
	lines = append(lines, fmt.Sprintf(`{"peer":"a","request":%q,"response":%q,"sent_at":%q,"received_at":%q}`,
		req, resp, sent.Format(time.RFC3339Nano), sent.Add(100*time.Millisecond).Format(time.RFC3339Nano)))
	// Duplicate of the same request.
	lines = append(lines, lines[0])
	// A kiss-of-death reply for a different request.
	sent2 := base.Add(time.Minute)
	xmit2 := stamp(sent2)
	req2 := pkt(0, 4, 3, 0, "", [8]byte{}, [8]byte{}, xmit2)
	resp2 := pkt(0, 4, 4, 0, "DENY", xmit2, stamp(sent2), stamp(sent2))
	lines = append(lines, fmt.Sprintf(`{"peer":"a","request":%q,"response":%q,"sent_at":%q,"received_at":%q}`,
		req2, resp2, sent2.Format(time.RFC3339Nano), sent2.Add(50*time.Millisecond).Format(time.RFC3339Nano)))
	// Garbage line.
	lines = append(lines, `{"peer":`)

	var out bytes.Buffer
	if err := run(strings.NewReader(strings.Join(lines, "\n")), &out, 8); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"VALID", "DUPLICATE", "kiss-of-death", "bad-json", "peer summaries", `peer "a": offset=`} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
}
