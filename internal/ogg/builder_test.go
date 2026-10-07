package ogg

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
)

// pageSpec describes a synthetic page for tests.
type pageSpec struct {
	headerType uint8
	granule    uint64
	serial     uint32
	seq        uint32
	segments   []uint8
	data       []byte
	badCRC     bool
}

func buildPage(p pageSpec) []byte {
	var b bytes.Buffer
	b.WriteString("OggS")
	b.WriteByte(0)
	b.WriteByte(p.headerType)
	binary.Write(&b, binary.LittleEndian, p.granule)
	binary.Write(&b, binary.LittleEndian, p.serial)
	binary.Write(&b, binary.LittleEndian, p.seq)
	crcPos := b.Len()
	b.Write([]byte{0, 0, 0, 0})
	b.WriteByte(byte(len(p.segments)))
	b.Write(p.segments)
	b.Write(p.data)
	raw := b.Bytes()
	crc := crc32.Checksum(raw, oggCRCTable)
	if p.badCRC {
		crc ^= 0xffffffff
	}
	binary.LittleEndian.PutUint32(raw[crcPos:], crc)
	return raw
}

// packetSegments splits a packet into lacing values, appending a zero
// terminator for lengths that are multiples of 255.
func packetSegments(data []byte) []uint8 {
	segs := []uint8{}
	n := len(data)
	for n >= 255 {
		segs = append(segs, 255)
		n -= 255
	}
	return append(segs, uint8(n))
}

// singlePacketPage builds a page carrying exactly one complete packet.
func singlePacketPage(ht uint8, granule uint64, serial, seq uint32, data []byte) pageSpec {
	return pageSpec{
		headerType: ht,
		granule:    granule,
		serial:     serial,
		seq:        seq,
		segments:   packetSegments(data),
		data:       data,
	}
}

func buildFile(specs ...pageSpec) []byte {
	var b bytes.Buffer
	for _, s := range specs {
		b.Write(buildPage(s))
	}
	return b.Bytes()
}

func collectPackets(t interface{ Fatalf(string, ...any) }, file []byte, opts AuditOptions) []*Packet {
	var pkts []*Packet
	err := Audit(NewReader(bytes.NewReader(file)), opts, func(p *Packet) error {
		cp := *p
		cp.Data = append([]byte(nil), p.Data...)
		pkts = append(pkts, &cp)
		return nil
	})
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	return pkts
}
