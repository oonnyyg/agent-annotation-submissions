package pack

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/binary"
	"strings"
	"testing"
)

// testEntry describes one entry for buildPack.
type testEntry struct {
	typ     ObjectType
	payload []byte // inflated payload (object body or delta data)
	baseOff int64  // for ObjOfsDelta: relative offset (positive = backwards)
	baseID  [20]byte
}

// buildPack assembles a valid v2 pack from entries and returns the bytes
// along with the absolute offset of each entry.
func buildPack(t *testing.T, entries []testEntry) ([]byte, []int64) {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("PACK")
	binary.Write(&buf, binary.BigEndian, uint32(2))
	binary.Write(&buf, binary.BigEndian, uint32(len(entries)))
	offsets := make([]int64, len(entries))
	for i, e := range entries {
		offsets[i] = int64(buf.Len())
		// Type + size varint.
		first := byte(e.typ<<4) | byte(len(e.payload)&0x0f)
		size := len(e.payload) >> 4
		var hdr []byte
		for {
			if size == 0 {
				hdr = append(hdr, first)
				break
			}
			hdr = append(hdr, first|0x80)
			first = byte(size & 0x7f)
			size >>= 7
		}
		buf.Write(hdr)
		switch e.typ {
		case ObjOfsDelta:
			// Encode relative offset (git's offset encoding).
			rel := e.baseOff
			var enc []byte
			enc = append(enc, byte(rel&0x7f))
			for rel >>= 7; rel != 0; rel >>= 7 {
				rel--
				enc = append([]byte{byte(rel&0x7f) | 0x80}, enc...)
			}
			buf.Write(enc)
		case ObjRefDelta:
			buf.Write(e.baseID[:])
		}
		var z bytes.Buffer
		zw := zlib.NewWriter(&z)
		zw.Write(e.payload)
		zw.Close()
		buf.Write(z.Bytes())
	}
	sum := sha1.Sum(buf.Bytes())
	buf.Write(sum[:])
	return buf.Bytes(), offsets
}

// makeDelta builds delta instructions: copy base[off:off+n] then insert lit.
func makeDelta(baseSize, resultSize int, lit []byte, copyOff, copyN int) []byte {
	var d []byte
	putVarint := func(v int) {
		for {
			b := byte(v & 0x7f)
			v >>= 7
			if v == 0 {
				d = append(d, b)
				return
			}
			d = append(d, b|0x80)
		}
	}
	putVarint(baseSize)
	putVarint(resultSize)
	if copyN > 0 {
		cmd := byte(0x80)
		var offBytes, sizeBytes [4]byte
		for i := 0; i < 4; i++ {
			if b := byte(copyOff >> uint(8*i)); b != 0 {
				cmd |= 1 << uint(i)
				offBytes[i] = b
			}
		}
		for i := 0; i < 3; i++ {
			if b := byte(copyN >> uint(8*i)); b != 0 {
				cmd |= 0x10 << uint(i)
				sizeBytes[i] = b
			}
		}
		d = append(d, cmd)
		for i := 0; i < 4; i++ {
			if cmd&(1<<uint(i)) != 0 {
				d = append(d, offBytes[i])
			}
		}
		for i := 0; i < 3; i++ {
			if cmd&(0x10<<uint(i)) != 0 {
				d = append(d, sizeBytes[i])
			}
		}
	}
	for len(lit) > 0 {
		n := len(lit)
		if n > 127 {
			n = 127
		}
		d = append(d, byte(n))
		d = append(d, lit[:n]...)
		lit = lit[n:]
	}
	return d
}

func TestResolveChainedAndForwardRef(t *testing.T) {
	base := []byte("hello world, this is the base blob content.")
	mid := append(append([]byte{}, base...), []byte(" +mid")...)
	final := append(append([]byte{}, mid...), []byte(" +final")...)

	midDelta := makeDelta(len(base), len(mid), []byte(" +mid"), 0, len(base))
	finalDelta := makeDelta(len(mid), len(final), []byte(" +final"), 0, len(mid))

	midID := ObjectID(ObjBlob, mid)
	refDelta := makeDelta(len(mid), len(mid)+3, []byte(" +r"), 0, len(mid))
	refResult := append(append([]byte{}, mid...), []byte(" +r")...)

	entries := []testEntry{
		{typ: ObjBlob, payload: base},
		{typ: ObjOfsDelta, payload: midDelta, baseOff: 0},   // filled below
		{typ: ObjOfsDelta, payload: finalDelta, baseOff: 0}, // filled below
		{typ: ObjRefDelta, payload: refDelta, baseID: midID},
	}
	// Iterate to a fixpoint: relative offsets depend on entry positions,
	// which shift as the offset varints change length.
	var data []byte
	var offsets []int64
	for i := 0; i < 10; i++ {
		data, offsets = buildPack(t, entries)
		entries[1].baseOff = offsets[1] - offsets[0]
		entries[2].baseOff = offsets[2] - offsets[1]
	}
	data, offsets = buildPack(t, entries)

	p, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	res, err := Resolve(p, DefaultLimits)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(res.Records) != 4 {
		t.Fatalf("got %d records, want 4", len(res.Records))
	}
	want := [][]byte{base, mid, final, refResult}
	for i, w := range want {
		id := ObjectID(ObjBlob, w)
		if res.Records[i].ID != id {
			t.Errorf("record %d: id %x, want %x", i, res.Records[i].ID, id)
		}
		if got := res.Objects[id]; !bytes.Equal(got, w) {
			t.Errorf("record %d: body mismatch", i)
		}
		if res.Records[i].Type != ObjBlob {
			t.Errorf("record %d: type %v, want blob", i, res.Records[i].Type)
		}
	}
	if !strings.Contains(res.Records[1].BaseDesc, "ofs-delta") ||
		!strings.Contains(res.Records[3].BaseDesc, "ref-delta") {
		t.Errorf("missing delta source descriptions: %+v", res.Records)
	}
}

func TestForwardRefDelta(t *testing.T) {
	// Ref-delta appears before its base in the pack.
	base := []byte("forward referenced base")
	baseID := ObjectID(ObjBlob, base)
	delta := makeDelta(len(base), len(base)+1, []byte("!"), 0, len(base))
	entries := []testEntry{
		{typ: ObjRefDelta, payload: delta, baseID: baseID},
		{typ: ObjBlob, payload: base},
	}
	data, _ := buildPack(t, entries)
	p, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	res, err := Resolve(p, DefaultLimits)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := append(append([]byte{}, base...), '!')
	if got := res.Objects[res.Records[0].ID]; !bytes.Equal(got, want) {
		t.Errorf("forward ref-delta body = %q, want %q", got, want)
	}
}
