package sfnt

import (
	"encoding/binary"
	"errors"
)

// cmapFormat4 implements the segment-mapping format 4 subtable,
// honoring relative idRangeOffset values and idDelta increments.
type cmapFormat4 struct {
	segCount    int
	endCode     []uint16
	startCode   []uint16
	idDelta     []int16
	idRangeOff  []uint16
	glyphIDBase int // offset of the glyphIdArray within the subtable
	data        []byte
}

func parseCmap4(data []byte) (*cmap, error) {
	if len(data) < 16 {
		return nil, errors.New("cmap format 4: header too small")
	}
	length := int(binary.BigEndian.Uint16(data[2:4]))
	if length > len(data) || length < 16 {
		return nil, errors.New("cmap format 4: declared length exceeds table")
	}
	data = data[:length]
	segCount := int(binary.BigEndian.Uint16(data[6:8]) / 2)
	if segCount == 0 || 16+segCount*8 > length {
		return nil, errors.New("cmap format 4: segment arrays exceed subtable length")
	}
	c4 := &cmapFormat4{
		segCount:    segCount,
		endCode:     make([]uint16, segCount),
		startCode:   make([]uint16, segCount),
		idDelta:     make([]int16, segCount),
		idRangeOff:  make([]uint16, segCount),
		glyphIDBase: 16 + segCount*8,
		data:        data,
	}
	for i := 0; i < segCount; i++ {
		c4.endCode[i] = binary.BigEndian.Uint16(data[14+i*2:])
		c4.startCode[i] = binary.BigEndian.Uint16(data[16+segCount*2+i*2:])
		c4.idDelta[i] = int16(binary.BigEndian.Uint16(data[16+segCount*4+i*2:]))
		c4.idRangeOff[i] = binary.BigEndian.Uint16(data[16+segCount*6+i*2:])
		if c4.endCode[i] < c4.startCode[i] {
			return nil, errors.New("cmap format 4: segment end before start")
		}
	}
	return &cmap{format4: c4}, nil
}

func (c4 *cmapFormat4) glyphID(r rune) uint16 {
	if r > 0xFFFF {
		return 0
	}
	c := uint16(r)
	for i := 0; i < c4.segCount; i++ {
		if c > c4.endCode[i] {
			continue
		}
		if c < c4.startCode[i] {
			return 0
		}
		if c4.idRangeOff[i] == 0 {
			return uint16(int32(c) + int32(c4.idDelta[i]))
		}
		// The offset is relative to the idRangeOffset word itself.
		pos := 16 + c4.segCount*6 + i*2 + int(c4.idRangeOff[i]) + int(c-c4.startCode[i])*2
		if pos+2 > len(c4.data) {
			return 0
		}
		g := binary.BigEndian.Uint16(c4.data[pos:])
		if g == 0 {
			return 0
		}
		return uint16(int32(g) + int32(c4.idDelta[i]))
	}
	return 0
}
