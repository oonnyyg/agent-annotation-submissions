package pack

import (
	"crypto/sha1"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func singleBlobPack(t *testing.T, body []byte) []byte {
	t.Helper()
	data, _ := buildPack(t, []testEntry{{typ: ObjBlob, payload: body}})
	return data
}

func TestRejectsCorruptPacks(t *testing.T) {
	good := singleBlobPack(t, []byte("payload"))

	cases := map[string][]byte{
		"bad magic":     func() []byte { d := append([]byte{}, good...); copy(d, "XXXX"); return d }(),
		"bad version":   func() []byte { d := append([]byte{}, good...); d[7] = 3; return d }(),
		"bad checksum":  func() []byte { d := append([]byte{}, good...); d[len(d)-1] ^= 0xff; return d }(),
		"truncated":     good[:len(good)-25],
		"trailing junk": withExtraByteBeforeTrailer(good),
	}
	for name, data := range cases {
		if _, err := Parse(data); err == nil {
			t.Errorf("%s: expected parse error", name)
		}
	}

	// Unknown object type 5, with a valid checksum.
	data, _ := buildPack(t, []testEntry{{typ: ObjectType(5), payload: []byte("x")}})
	if _, err := Parse(data); err == nil || !strings.Contains(err.Error(), "unknown object type") {
		t.Errorf("unknown type: got %v", err)
	}

	// Declared size does not match inflated size: rebuild with a tweaked
	// header by hand.
	d := singleBlobPack(t, []byte("payload"))
	d[12] = d[12]&0xf0 | 0x05 // claim size 5 instead of 7
	// Fix checksum so only the size check can fail.
	sum := sha1.Sum(d[:len(d)-20])
	copy(d[len(d)-20:], sum[:])
	if _, err := Parse(d); err == nil || !strings.Contains(err.Error(), "declared size") {
		t.Errorf("size mismatch: got %v", err)
	}
}

// withExtraByteBeforeTrailer inserts one padding byte before the checksum
// and recomputes it, so only the trailing-byte check can fail.
func withExtraByteBeforeTrailer(pack []byte) []byte {
	body := append([]byte{}, pack[:len(pack)-20]...)
	body = append(body, 0x00)
	sum := sha1.Sum(body)
	return append(body, sum[:]...)
}

func TestMissingRefBase(t *testing.T) {
	var id [20]byte
	id[0] = 0xde
	delta := makeDelta(3, 3, []byte("abc"), 0, 0)
	data, _ := buildPack(t, []testEntry{{typ: ObjRefDelta, payload: delta, baseID: id}})
	p, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	_, err = Resolve(p, DefaultLimits)
	if err == nil || !strings.Contains(err.Error(), "not found in pack") {
		t.Fatalf("expected missing base error, got %v", err)
	}
	if !strings.Contains(err.Error(), "offset 12") {
		t.Errorf("error should name the dependent entry: %v", err)
	}
}

func TestDeltaViolations(t *testing.T) {
	base := []byte("abc")
	cases := map[string][]byte{
		"copy out of range":  makeDelta(3, 4, nil, 0, 4),
		"base size mismatch": makeDelta(2, 3, nil, 0, 3),
		"reserved opcode":    append(append([]byte{3, 3}, 0x00), 0x81, 0x01, 0x03),
		"truncated insert":   {3, 3, 0x05, 'a', 'b'},
		"result overrun":     {3, 2, 0x83, 0x00, 0x03},
	}
	for name, delta := range cases {
		if _, err := ApplyDelta(base, delta); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	// A valid delta still works.
	out, err := ApplyDelta(base, makeDelta(3, 5, []byte("de"), 0, 3))
	if err != nil || string(out) != "abcde" {
		t.Errorf("valid delta: %q, %v", out, err)
	}
}

func TestLimits(t *testing.T) {
	base := []byte("chain base")
	d1 := makeDelta(len(base), len(base)+1, []byte("1"), 0, len(base))
	mid := append(append([]byte{}, base...), '1')
	d2 := makeDelta(len(mid), len(mid)+1, []byte("2"), 0, len(mid))
	entries := []testEntry{
		{typ: ObjBlob, payload: base},
		{typ: ObjOfsDelta, payload: d1},
		{typ: ObjOfsDelta, payload: d2},
	}
	var data []byte
	var offsets []int64
	for i := 0; i < 10; i++ {
		data, offsets = buildPack(t, entries)
		entries[1].baseOff = offsets[1] - offsets[0]
		entries[2].baseOff = offsets[2] - offsets[1]
	}
	data, _ = buildPack(t, entries)
	p, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, err := Resolve(p, Limits{MaxObjectSize: 1 << 20, MaxDeltaDepth: 1}); err == nil ||
		!strings.Contains(err.Error(), "delta depth") {
		t.Errorf("depth limit: got %v", err)
	}
	if _, err := Resolve(p, Limits{MaxObjectSize: 5, MaxDeltaDepth: 10}); err == nil ||
		!strings.Contains(err.Error(), "object size") {
		t.Errorf("size limit: got %v", err)
	}
	if _, err := Resolve(p, Limits{MaxObjectSize: 1 << 20, MaxDeltaDepth: 10}); err != nil {
		t.Errorf("within limits: got %v", err)
	}
}

func TestExport(t *testing.T) {
	body := []byte("export me")
	data := singleBlobPack(t, body)
	p, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	res, err := Resolve(p, DefaultLimits)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	if err := res.Export(out); err != nil {
		t.Fatalf("Export: %v", err)
	}
	id := ObjectID(ObjBlob, body)
	got, err := os.ReadFile(filepath.Join(out, "objects", hexID(id)))
	if err != nil || string(got) != string(body) {
		t.Errorf("object file: %q, %v", got, err)
	}
	man, err := os.ReadFile(filepath.Join(out, "MANIFEST.txt"))
	if err != nil || !strings.Contains(string(man), "blob") {
		t.Errorf("manifest: %q, %v", man, err)
	}
	// Second export must refuse to overwrite.
	if err := res.Export(out); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("overwrite: got %v", err)
	}
}

func hexID(id [20]byte) string {
	const hexdig = "0123456789abcdef"
	var b [40]byte
	for i, c := range id {
		b[2*i] = hexdig[c>>4]
		b[2*i+1] = hexdig[c&0xf]
	}
	return string(b[:])
}
