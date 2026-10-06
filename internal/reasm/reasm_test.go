package reasm

import (
	"bytes"
	"testing"

	"example.com/pcap-fragment-audit/internal/parse"
	"example.com/pcap-fragment-audit/internal/testutil"
)

var (
	src = [4]byte{192, 168, 1, 10}
	dst = [4]byte{192, 168, 1, 20}
)

// feed decodes and adds fragments, returning the group's final result.
func feed(t *testing.T, frags ...testutil.FragmentParams) *Result {
	t.Helper()
	r := New()
	for i, fp := range frags {
		if fp.Src == ([4]byte{}) {
			fp.Src, fp.Dst = src, dst
		}
		pkt, err := parse.Decode(testutil.BuildFragment(fp))
		if err != nil {
			t.Fatalf("decode fragment %d: %v", i, err)
		}
		r.Add(pkt, i+1)
	}
	res := r.Finalize()
	if len(res) != 1 {
		t.Fatalf("expected 1 group, got %d", len(res))
	}
	return res[0]
}

func udpBytes(payload []byte) []byte {
	// mirror testutil's UDP header construction for verification
	f := testutil.BuildFragment(testutil.FragmentParams{
		Src: src, Dst: dst, UDPSrc: 5000, UDPDst: 5001, Payload: payload,
	})
	return f[14+20:]
}

func TestOutOfOrderReassembly(t *testing.T) {
	payload := []byte("0123456789abcdef") // 16 bytes -> two 8-byte frags after header
	full := udpBytes(payload)
	res := feed(t,
		testutil.FragmentParams{Ident: 100, FragOff: 16, MoreFrag: false, Raw: full[16:]},
		testutil.FragmentParams{Ident: 100, FragOff: 0, MoreFrag: true, Raw: full[:16]},
	)
	if res.Conflict || !res.Complete {
		t.Fatalf("expected complete, got %+v gaps=%v", res, res.Gaps)
	}
	if !bytes.Equal(res.UDP, full) {
		t.Fatal("reassembled bytes mismatch")
	}
}

func TestIdenticalOverlapCountedOnce(t *testing.T) {
	payload := []byte("0123456789abcdef")
	full := udpBytes(payload)
	// second fragment overlaps 8 identical bytes with the first
	res := feed(t,
		testutil.FragmentParams{Ident: 101, FragOff: 0, MoreFrag: true, Raw: full[:16]},
		testutil.FragmentParams{Ident: 101, FragOff: 8, MoreFrag: true, Raw: full[8:24]},
		testutil.FragmentParams{Ident: 101, FragOff: 16, MoreFrag: false, Raw: full[16:24]},
	)
	if res.Conflict {
		t.Fatal("identical overlap must not be a conflict")
	}
	if !res.Complete || !bytes.Equal(res.UDP, full) {
		t.Fatalf("expected complete reassembly, got %+v", res)
	}
}

func TestConflictingOverlap(t *testing.T) {
	payload := []byte("0123456789abcdef")
	full := udpBytes(payload)
	bad := append([]byte(nil), full[8:24]...)
	bad[0] ^= 0xff // differ inside the overlap
	res := feed(t,
		testutil.FragmentParams{Ident: 102, FragOff: 0, MoreFrag: true, Raw: full[:16]},
		testutil.FragmentParams{Ident: 102, FragOff: 8, MoreFrag: true, Raw: bad},
		testutil.FragmentParams{Ident: 102, FragOff: 16, MoreFrag: false, Raw: full[16:24]},
	)
	if !res.Conflict {
		t.Fatal("differing overlap must be a conflict")
	}
	if res.Complete {
		t.Fatal("conflicted group must not complete")
	}
}

func TestLastFragmentLengthConflict(t *testing.T) {
	payload := []byte("0123456789abcdef")
	full := udpBytes(payload)
	res := feed(t,
		testutil.FragmentParams{Ident: 103, FragOff: 0, MoreFrag: true, Raw: full[:16]},
		testutil.FragmentParams{Ident: 103, FragOff: 16, MoreFrag: false, Raw: full[16:24]},
		testutil.FragmentParams{Ident: 103, FragOff: 16, MoreFrag: false, Raw: full[16:20]}, // shorter last
	)
	if !res.Conflict {
		t.Fatal("conflicting last-fragment lengths must be a conflict")
	}
}

func TestMissingFragmentGaps(t *testing.T) {
	payload := []byte("0123456789abcdef")
	full := udpBytes(payload)
	res := feed(t,
		testutil.FragmentParams{Ident: 104, FragOff: 0, MoreFrag: true, Raw: full[:8]},
		testutil.FragmentParams{Ident: 104, FragOff: 16, MoreFrag: false, Raw: full[16:24]},
	)
	if res.Complete {
		t.Fatal("must not complete with a gap")
	}
	if len(res.Gaps) != 1 || res.Gaps[0].Start != 8 || res.Gaps[0].End != 16 {
		t.Fatalf("bad gaps: %+v", res.Gaps)
	}
	if !res.LastSeen {
		t.Fatal("last fragment was seen")
	}
}

func TestLastFragmentUnknown(t *testing.T) {
	full := udpBytes([]byte("0123456789abcdef"))
	res := feed(t,
		testutil.FragmentParams{Ident: 105, FragOff: 0, MoreFrag: true, Raw: full[:16]},
	)
	if res.Complete || res.LastSeen {
		t.Fatalf("expected incomplete with unknown last, got %+v", res)
	}
	if len(res.Gaps) == 0 || res.Gaps[len(res.Gaps)-1].End != -1 {
		t.Fatalf("expected open-ended gap, got %+v", res.Gaps)
	}
}

func TestUnfragmentedDatagram(t *testing.T) {
	payload := []byte("hello world")
	res := feed(t,
		testutil.FragmentParams{Ident: 106, UDPSrc: 5000, UDPDst: 5001, Payload: payload},
	)
	if !res.Complete {
		t.Fatalf("unfragmented datagram should complete: %+v", res)
	}
	if !bytes.Equal(res.UDP[8:], payload) {
		t.Fatal("payload mismatch")
	}
}

func TestNonLastNotMultipleOfEight(t *testing.T) {
	full := udpBytes([]byte("0123456789abcdef"))
	res := feed(t,
		testutil.FragmentParams{Ident: 107, FragOff: 0, MoreFrag: true, Raw: full[:13]},
		testutil.FragmentParams{Ident: 107, FragOff: 16, MoreFrag: false, Raw: full[16:24]},
	)
	if !res.Conflict {
		t.Fatal("non-last fragment with unaligned payload must be a conflict")
	}
}
