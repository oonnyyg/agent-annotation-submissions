package pack

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
)

// testEntry describes one entry for buildPack.
type testEntry struct {
	typ      int
	data     []byte // inflated payload
	baseRel  int64  // ofs-delta: distance back to base entry
	baseID   string // ref-delta: hex id of base
	declSize int64  // override declared size; 0 means len(data)
}

// buildPack assembles a valid v2 pack from entries.
func buildPack(t *testing.T, entries []testEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("PACK")
	binary.Write(&buf, binary.BigEndian, uint32(2))
	binary.Write(&buf, binary.BigEndian, uint32(len(entries)))
	for _, e := range entries {
		size := e.declSize
		if size == 0 {
			size = int64(len(e.data))
		}
		writeEntryHeader(&buf, e.typ, size)
		switch e.typ {
		case TypeOfsDelta:
			writeOfsDeltaRef(&buf, e.baseRel)
		case TypeRefDelta:
			id, err := decodeHex(e.baseID)
			if err != nil {
				t.Fatalf("bad base id: %v", err)
			}
			buf.Write(id)
		}
		var z bytes.Buffer
		zw := zlib.NewWriter(&z)
		zw.Write(e.data)
		zw.Close()
		buf.Write(z.Bytes())
	}
	sum := sha1.Sum(buf.Bytes())
	buf.Write(sum[:])
	return buf.Bytes()
}

func writeEntryHeader(buf *bytes.Buffer, typ int, size int64) {
	b := byte(typ<<4) | byte(size&0x0f)
	size >>= 4
	for size > 0 {
		buf.WriteByte(b | 0x80)
		b = byte(size & 0x7f)
		size >>= 7
	}
	buf.WriteByte(b)
}

func writeOfsDeltaRef(buf *bytes.Buffer, rel int64) {
	var tmp [16]byte
	i := len(tmp) - 1
	tmp[i] = byte(rel & 0x7f)
	for rel >>= 7; rel > 0; rel >>= 7 {
		rel--
		i--
		tmp[i] = byte(rel&0x7f) | 0x80
	}
	buf.Write(tmp[i:])
}

func decodeHex(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("odd hex length")
	}
	out := make([]byte, len(s)/2)
	for i := range out {
		var v int
		if _, err := fmt.Sscanf(s[2*i:2*i+2], "%02x", &v); err != nil {
			return nil, err
		}
		out[i] = byte(v)
	}
	return out, nil
}

// makeDelta builds delta instructions turning base into result.
// It emits one insert of the whole result (sufficient for tests that
// only need size bookkeeping) unless copyFrom is non-negative, in which
// case it first copies copyLen bytes from base at copyFrom.
func makeDelta(baseLen int, result []byte, copyFrom, copyLen int) []byte {
	var buf bytes.Buffer
	writeDeltaVarint(&buf, int64(baseLen))
	writeDeltaVarint(&buf, int64(len(result)))
	if copyFrom >= 0 {
		op := byte(0x80)
		var offBytes, sizeBytes [4]byte
		nOff, nSize := 0, 0
		for i := 0; i < 4; i++ {
			if copyFrom&(0xff<<(8*uint(i))) != 0 {
				op |= 1 << uint(i)
				offBytes[nOff] = byte(copyFrom >> (8 * uint(i)))
				nOff++
			}
		}
		for i := 0; i < 3; i++ {
			if copyLen&(0xff<<(8*uint(i))) != 0 {
				op |= 0x10 << uint(i)
				sizeBytes[nSize] = byte(copyLen >> (8 * uint(i)))
				nSize++
			}
		}
		buf.WriteByte(op)
		buf.Write(offBytes[:nOff])
		buf.Write(sizeBytes[:nSize])
	}
	rest := result
	if copyFrom >= 0 {
		rest = result[copyLen:]
	}
	for len(rest) > 0 {
		n := len(rest)
		if n > 127 {
			n = 127
		}
		buf.WriteByte(byte(n))
		buf.Write(rest[:n])
		rest = rest[n:]
	}
	return buf.Bytes()
}

func writeDeltaVarint(buf *bytes.Buffer, v int64) {
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v == 0 {
			buf.WriteByte(b)
			return
		}
		buf.WriteByte(b | 0x80)
	}
}

func TestParsePlainObjects(t *testing.T) {
	blob := []byte("hello pack\n")
	commit := []byte("tree abc\nauthor x\n\nmsg\n")
	data := buildPack(t, []testEntry{
		{typ: TypeBlob, data: blob},
		{typ: TypeCommit, data: commit},
		{typ: TypeTree, data: []byte("tree-bytes")},
		{typ: TypeTag, data: []byte("tag-bytes")},
	})
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if f.Declared != 4 || len(f.Entries) != 4 {
		t.Fatalf("want 4 entries, got %d/%d", f.Declared, len(f.Entries))
	}
	objs, err := f.Resolve(DefaultLimits)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if objs[0].Type != TypeBlob || !bytes.Equal(objs[0].Data, blob) {
		t.Fatalf("blob mismatch")
	}
	if objs[0].ID != ObjectID(TypeBlob, blob) {
		t.Fatalf("id mismatch")
	}
	if objs[0].Entry.Offset != 12 {
		t.Fatalf("first entry offset = %d, want 12", objs[0].Entry.Offset)
	}
}

func TestOfsDeltaChain(t *testing.T) {
	base := []byte("base content for delta chain")
	mid := []byte("base content for delta CHAIN!")
	leaf := []byte("LEAF content for delta CHAIN!")
	entries := []testEntry{
		{typ: TypeBlob, data: base},
		{typ: TypeOfsDelta, data: makeDelta(len(base), mid, 0, 23), baseRel: 1},
		{typ: TypeOfsDelta, data: makeDelta(len(mid), leaf, -1, 0), baseRel: 1},
	}
	// Second pass: fix up relative offsets using real entry offsets.
	f, err := Parse(buildPack(t, entries))
	if err != nil {
		t.Fatalf("Parse probe: %v", err)
	}
	entries[1].baseRel = f.Entries[1].Offset - f.Entries[0].Offset
	entries[2].baseRel = f.Entries[2].Offset - f.Entries[1].Offset
	f, err = Parse(buildPack(t, entries))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	objs, err := f.Resolve(DefaultLimits)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !bytes.Equal(objs[1].Data, mid) || !bytes.Equal(objs[2].Data, leaf) {
		t.Fatalf("delta chain reconstruction failed: %q / %q", objs[1].Data, objs[2].Data)
	}
	if objs[2].Type != TypeBlob || objs[2].Depth != 2 {
		t.Fatalf("leaf type/depth wrong: %d/%d", objs[2].Type, objs[2].Depth)
	}
	if objs[2].ID != ObjectID(TypeBlob, leaf) {
		t.Fatalf("leaf id mismatch")
	}
	if objs[1].BaseFrom == "" || objs[2].BaseFrom == "" {
		t.Fatalf("delta base provenance missing")
	}
}

func TestRefDeltaBaseAppearsLater(t *testing.T) {
	base := []byte("later base object")
	result := []byte("later base object plus")
	baseID := ObjectID(TypeBlob, base)
	data := buildPack(t, []testEntry{
		{typ: TypeRefDelta, data: makeDelta(len(base), result, 0, len(base)), baseID: baseID},
		{typ: TypeBlob, data: base},
	})
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	objs, err := f.Resolve(DefaultLimits)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !bytes.Equal(objs[0].Data, result) {
		t.Fatalf("ref-delta result = %q", objs[0].Data)
	}
	if objs[0].BaseFrom != "ref:"+baseID {
		t.Fatalf("base provenance = %q", objs[0].BaseFrom)
	}
}

func TestRejectCorruptPacks(t *testing.T) {
	valid := buildPack(t, []testEntry{{typ: TypeBlob, data: []byte("x")}})

	t.Run("checksum", func(t *testing.T) {
		bad := append([]byte(nil), valid...)
		bad[len(bad)-1] ^= 0xff
		if _, err := Parse(bad); err == nil || !strings.Contains(err.Error(), "checksum") {
			t.Fatalf("want checksum error, got %v", err)
		}
	})
	t.Run("truncated", func(t *testing.T) {
		short := append([]byte(nil), valid[:len(valid)-checksumSize-3]...)
		sum := sha1.Sum(short)
		short = append(short, sum[:]...)
		if _, err := Parse(short); err == nil {
			t.Fatalf("want truncation error")
		}
	})
	t.Run("trailing bytes", func(t *testing.T) {
		body := append([]byte(nil), valid[:len(valid)-checksumSize]...)
		body = append(body, 0x00, 0x01)
		sum := sha1.Sum(body)
		if _, err := Parse(append(body, sum[:]...)); err == nil ||
			!strings.Contains(err.Error(), "trailing") {
			t.Fatalf("want trailing-bytes error, got %v", err)
		}
	})
	t.Run("unknown type", func(t *testing.T) {
		data := buildPack(t, []testEntry{{typ: 5, data: []byte("x")}})
		if _, err := Parse(data); err == nil || !strings.Contains(err.Error(), "unknown object type") {
			t.Fatalf("want unknown-type error, got %v", err)
		}
	})
	t.Run("bad version", func(t *testing.T) {
		bad := append([]byte(nil), valid...)
		bad[7] = 3
		sum := sha1.Sum(bad[:len(bad)-checksumSize])
		copy(bad[len(bad)-checksumSize:], sum[:])
		if _, err := Parse(bad); err == nil || !strings.Contains(err.Error(), "version") {
			t.Fatalf("want version error, got %v", err)
		}
	})
	t.Run("declared size mismatch", func(t *testing.T) {
		data := buildPack(t, []testEntry{{typ: TypeBlob, data: []byte("hello"), declSize: 4}})
		if _, err := Parse(data); err == nil {
			t.Fatalf("want size mismatch error")
		}
	})
}

func TestRejectBadDependencies(t *testing.T) {
	t.Run("missing ofs base", func(t *testing.T) {
		data := buildPack(t, []testEntry{
			{typ: TypeBlob, data: []byte("base")},
			{typ: TypeOfsDelta, data: makeDelta(4, []byte("zzzz"), 0, 4), baseRel: 1},
		})
		f, err := Parse(data)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		// Point the delta one byte before the real base entry.
		f.Entries[1].BaseOffset = f.Entries[0].Offset - 1
		if _, err := f.Resolve(DefaultLimits); err == nil ||
			!strings.Contains(err.Error(), "not in pack") {
			t.Fatalf("want missing-base error, got %v", err)
		}
	})
	t.Run("missing ref base", func(t *testing.T) {
		data := buildPack(t, []testEntry{
			{typ: TypeRefDelta, data: makeDelta(4, []byte("zzzz"), 0, 4),
				baseID: "0000000000000000000000000000000000000000"},
			{typ: TypeBlob, data: []byte("unrelated")},
		})
		f, err := Parse(data)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		_, err = f.Resolve(DefaultLimits)
		if err == nil || !strings.Contains(err.Error(), "not found in pack") {
			t.Fatalf("want missing-ref-base error, got %v", err)
		}
		if !strings.Contains(err.Error(), "offset 12") {
			t.Fatalf("error should name dependent entry offset, got %v", err)
		}
	})
}
