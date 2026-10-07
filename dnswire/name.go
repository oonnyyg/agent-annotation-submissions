package dnswire

const (
	maxNameLen  = 255
	maxLabelLen = 63
	ptrMask     = 0xC0
	maxPtrOff   = 0x3FFF
)

// Name is a domain name as a sequence of ASCII labels. The root name is
// an empty slice. Label case is preserved.
type Name []string

// NameResult reports both the wire bytes a name occupies at its position
// and the fully expanded length.
type NameResult struct {
	Labels        []string `json:"labels"`
	OccupiedBytes int      `json:"occupiedBytes"`
	ExpandedLen   int      `json:"expandedLen"`
}

// readName decodes a possibly compressed name starting at offset. It
// follows multi-level pointers for expansion but reports the number of
// bytes the name occupies at its original position, so the caller can
// continue reading subsequent fields correctly.
func readName(msg []byte, offset int) (NameResult, error) {
	var labels []string
	occupied := -1
	expanded := 0
	pos := offset
	jumped := false
	seen := map[int]bool{}

	for {
		if pos >= len(msg) {
			return NameResult{}, errAt(pos, "name truncated: label byte outside message")
		}
		b := msg[pos]
		switch b & ptrMask {
		case ptrMask:
			if pos+1 >= len(msg) {
				return NameResult{}, errAt(pos, "compression pointer truncated")
			}
			ptr := int(b&^ptrMask)<<8 | int(msg[pos+1])
			if ptr >= len(msg) {
				return NameResult{}, errAt(pos, "compression pointer %d out of bounds (message %d bytes)", ptr, len(msg))
			}
			if seen[ptr] {
				return NameResult{}, errAt(pos, "compression pointer loop at offset %d", ptr)
			}
			seen[ptr] = true
			if !jumped {
				occupied = pos + 2 - offset
				jumped = true
			}
			pos = ptr
		case 0:
			if b == 0 {
				if !jumped {
					occupied = pos + 1 - offset
				}
				expanded++
				if expanded > maxNameLen {
					return NameResult{}, errAt(offset, "expanded name exceeds %d bytes", maxNameLen)
				}
				return NameResult{Labels: labels, OccupiedBytes: occupied, ExpandedLen: expanded}, nil
			}
			if int(b) > maxLabelLen {
				return NameResult{}, errAt(pos, "label length %d exceeds %d", b, maxLabelLen)
			}
			if pos+1+int(b) > len(msg) {
				return NameResult{}, errAt(pos, "label of %d bytes truncated", b)
			}
			label := string(msg[pos+1 : pos+1+int(b)])
			for i := 0; i < len(label); i++ {
				if label[i] >= 0x80 {
					return NameResult{}, errAt(pos+1+i, "label contains non-ASCII byte 0x%02x", label[i])
				}
			}
			labels = append(labels, label)
			expanded += 1 + int(b)
			if expanded > maxNameLen {
				return NameResult{}, errAt(offset, "expanded name exceeds %d bytes", maxNameLen)
			}
			if !jumped {
				seen[pos] = true
			}
			pos += 1 + int(b)
		default:
			return NameResult{}, errAt(pos, "reserved label type 0x%02x", b&ptrMask)
		}
	}
}

// nameEncoder tracks suffix offsets for compression while encoding.
type nameEncoder struct {
	suffixes map[string]int
}

func newNameEncoder() *nameEncoder {
	return &nameEncoder{suffixes: map[string]int{}}
}

// appendName encodes name into buf, reusing previously encoded suffixes
// via compression pointers. Pointers are only emitted to offsets that fit
// in 14 bits and point at real label boundaries.
func (e *nameEncoder) appendName(buf []byte, name Name) ([]byte, error) {
	expanded := 1
	for _, l := range name {
		if len(l) == 0 {
			return nil, errAt(len(buf), "empty label in name")
		}
		if len(l) > maxLabelLen {
			return nil, errAt(len(buf), "label %q exceeds %d bytes", l, maxLabelLen)
		}
		for i := 0; i < len(l); i++ {
			if l[i] >= 0x80 {
				return nil, errAt(len(buf), "label %q contains non-ASCII byte", l)
			}
		}
		expanded += 1 + len(l)
	}
	if expanded > maxNameLen {
		return nil, errAt(len(buf), "name exceeds %d bytes", maxNameLen)
	}

	for i := 0; i < len(name); i++ {
		key := suffixKey(name[i:])
		if off, ok := e.suffixes[key]; ok && off <= maxPtrOff {
			return append(buf, byte(ptrMask|off>>8), byte(off)), nil
		}
		if len(buf) <= maxPtrOff {
			e.suffixes[key] = len(buf)
		}
		buf = append(buf, byte(len(name[i])))
		buf = append(buf, name[i]...)
	}
	return append(buf, 0), nil
}

func suffixKey(labels []string) string {
	key := ""
	for _, l := range labels {
		key += l + "\x00"
	}
	return key
}
