package oggxport

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"

	"example.com/ogg-packet-audit/internal/ogg"
)

var testCRCTable = crc32.MakeTable(0x04c11db7)

func mkPage(ht uint8, granule uint64, serial, seq uint32, data []byte, corrupt bool) []byte {
	var b bytes.Buffer
	b.WriteString("OggS")
	b.WriteByte(0)
	b.WriteByte(ht)
	binary.Write(&b, binary.LittleEndian, granule)
	binary.Write(&b, binary.LittleEndian, serial)
	binary.Write(&b, binary.LittleEndian, seq)
	crcPos := b.Len()
	b.Write([]byte{0, 0, 0, 0})
	segs := []uint8{}
	for n := len(data); ; {
		if n >= 255 {
			segs = append(segs, 255)
			n -= 255
			continue
		}
		segs = append(segs, uint8(n))
		break
	}
	b.WriteByte(byte(len(segs)))
	b.Write(segs)
	b.Write(data)
	raw := b.Bytes()
	crc := crc32.Checksum(raw, testCRCTable)
	if corrupt {
		crc ^= 1
	}
	binary.LittleEndian.PutUint32(raw[crcPos:], crc)
	return raw
}

func validFile() []byte {
	return bytes.Join([][]byte{
		mkPage(ogg.FlagBOS, 0, 0x10, 0, []byte("alpha"), false),
		mkPage(0, 3, 0x10, 1, []byte("beta"), false),
		mkPage(ogg.FlagEOS, 6, 0x10, 2, nil, false),
	}, nil)
}

func TestExportSuccess(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out")
	rep, err := Export(bytes.NewReader(validFile()), "in.ogg", out, ogg.AuditOptions{})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if rep.Streams != 1 || rep.Packets != 3 || rep.Pages != 3 {
		t.Fatalf("report = %+v", rep)
	}
	data, err := os.ReadFile(filepath.Join(out, "stream-00000010", "packet-000001.bin"))
	if err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if string(data) != "beta" {
		t.Fatalf("packet bytes = %q", data)
	}
	raw, err := os.ReadFile(filepath.Join(out, "index.json"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	var idx Index
	if err := json.Unmarshal(raw, &idx); err != nil {
		t.Fatalf("parse index: %v", err)
	}
	if len(idx.Packets) != 3 || idx.Packets[1].Length != 4 || idx.Packets[1].GranulePos != 3 {
		t.Fatalf("bad index entry: %+v", idx.Packets[1])
	}
	if len(idx.Packets[1].Pages) != 1 || idx.Packets[1].Pages[0].Offset == 0 {
		t.Fatalf("bad page refs: %+v", idx.Packets[1].Pages)
	}
	if idx.Packets[2].Length != 0 {
		t.Fatalf("empty packet length = %d", idx.Packets[2].Length)
	}
	if idx.PageCnt != 3 {
		t.Fatalf("page_count = %d", idx.PageCnt)
	}
}

func TestExportFailureKeepsExistingOutput(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(out, "previous.txt")
	if err := os.WriteFile(marker, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	file := bytes.Join([][]byte{
		mkPage(ogg.FlagBOS, 0, 0x10, 0, []byte("alpha"), false),
		mkPage(ogg.FlagEOS, 6, 0x10, 1, nil, true),
	}, nil)
	if _, err := Export(bytes.NewReader(file), "bad.ogg", out, ogg.AuditOptions{}); err == nil {
		t.Fatal("expected export to fail")
	}
	got, err := os.ReadFile(marker)
	if err != nil || string(got) != "keep me" {
		t.Fatalf("existing output was modified: %v %q", err, got)
	}
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("unexpected new entries in output dir: %d", len(entries))
	}
	siblings, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range siblings {
		if e.Name() != "out" {
			t.Fatalf("leftover staging entry: %s", e.Name())
		}
	}
}
