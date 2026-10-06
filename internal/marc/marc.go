// Package marc parses consecutive MARC21 binary records (UTF-8 only).
package marc

import (
	"fmt"
	"strconv"
	"unicode/utf8"
)

const (
	fieldTerminator  = 0x1E
	recordTerminator = 0x1D
	subfieldDelim    = 0x1F
	leaderLen        = 24
)

// Subfield is one MARC subfield; Code is the byte after the delimiter.
type Subfield struct {
	Code  byte
	Value string
}

// Field is one MARC field. Control fields (tag < 010) keep their raw
// value and are never split into subfields.
type Field struct {
	Tag       string
	Value     string // control fields only
	Subfields []Subfield
}

// Record is a parsed MARC record. Fields preserve physical order.
type Record struct {
	Index  int // 0-based position in the stream
	Offset int // byte offset of the record in the stream
	Fields []Field
}

// FieldsByTag returns all fields with the given tag, in order.
func (r *Record) FieldsByTag(tag string) []Field {
	var out []Field
	for _, f := range r.Fields {
		if f.Tag == tag {
			out = append(out, f)
		}
	}
	return out
}

// ParseError describes a structural problem. Fatal errors stop parsing;
// non-fatal ones isolate a bad record and parsing continues.
type ParseError struct {
	Offset  int
	Message string
	Fatal   bool
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("offset %d: %s", e.Offset, e.Message)
}

// Parse reads consecutive MARC21 records from data. It returns the parsed
// records plus any diagnostics; a fatal diagnostic ends the stream.
func Parse(data []byte) ([]Record, []*ParseError) {
	var records []Record
	var diags []*ParseError
	pos := 0
	for pos < len(data) {
		rec, next, perr := parseOne(data, pos, len(records))
		if perr != nil {
			diags = append(diags, perr)
			if perr.Fatal {
				break
			}
			pos = next
			continue
		}
		records = append(records, *rec)
		pos = next
	}
	return records, diags
}

// parseOne parses the record starting at pos. On a recoverable error it
// returns the byte position just past the bad record so parsing continues.
func parseOne(data []byte, pos, index int) (*Record, int, *ParseError) {
	remaining := len(data) - pos
	if remaining < leaderLen {
		return nil, 0, &ParseError{Offset: pos, Fatal: true,
			Message: fmt.Sprintf("remaining %d bytes shorter than 24-byte leader", remaining)}
	}
	leader := data[pos : pos+leaderLen]
	recLen, err := strconv.Atoi(string(leader[0:5]))
	if err != nil || recLen < leaderLen+2 || recLen > remaining {
		return nil, 0, &ParseError{Offset: pos, Fatal: true,
			Message: fmt.Sprintf("invalid record length %q (need integer, >= 26, <= remaining %d bytes)", string(leader[0:5]), remaining)}
	}
	raw := data[pos : pos+recLen]
	end := pos + recLen

	isolate := func(msg string) (*Record, int, *ParseError) {
		return nil, end, &ParseError{Offset: pos, Message: msg}
	}

	if raw[recLen-1] != recordTerminator {
		return isolate(fmt.Sprintf("record of %d bytes lacks record terminator", recLen))
	}
	baseAddr, err := strconv.Atoi(string(leader[12:17]))
	if err != nil || baseAddr < leaderLen+1 || baseAddr > recLen {
		return isolate(fmt.Sprintf("invalid base address %q", string(leader[12:17])))
	}
	dir := raw[leaderLen:baseAddr]
	if len(dir) == 0 || dir[len(dir)-1] != fieldTerminator {
		return isolate("directory not terminated by field terminator")
	}
	dir = dir[:len(dir)-1]
	if len(dir)%12 != 0 {
		return isolate(fmt.Sprintf("directory size %d not a multiple of 12", len(dir)))
	}

	rec := &Record{Index: index, Offset: pos}
	for i := 0; i < len(dir); i += 12 {
		entry := dir[i : i+12]
		tag := string(entry[0:3])
		for _, c := range []byte(tag) {
			if c < '0' || c > '9' {
				return isolate(fmt.Sprintf("directory entry %d has non-numeric tag %q", i/12, tag))
			}
		}
		flen, err1 := strconv.Atoi(string(entry[3:7]))
		foff, err2 := strconv.Atoi(string(entry[7:12]))
		if err1 != nil || err2 != nil {
			return isolate(fmt.Sprintf("directory entry %d has non-numeric length/offset", i/12))
		}
		start := baseAddr + foff
		stop := start + flen
		if flen < 1 || start < baseAddr || stop > recLen-1 {
			return isolate(fmt.Sprintf("field %s range [%d,%d) outside record data", tag, start, stop))
		}
		body := raw[start:stop]
		if body[flen-1] != fieldTerminator {
			return isolate(fmt.Sprintf("field %s not terminated by field terminator", tag))
		}
		body = body[:flen-1]
		if !utf8.Valid(body) {
			return isolate(fmt.Sprintf("field %s contains invalid UTF-8", tag))
		}
		rec.Fields = append(rec.Fields, makeField(tag, body))
	}
	return rec, end, nil
}

// makeField builds a Field. Control fields (tag < "010") keep the raw
// value; data fields are split into subfields preserving order.
func makeField(tag string, body []byte) Field {
	f := Field{Tag: tag}
	if tag < "010" {
		f.Value = string(body)
		return f
	}
	// Skip the two indicator bytes when present.
	rest := body
	if len(rest) >= 2 {
		rest = rest[2:]
	}
	for len(rest) > 0 {
		if rest[0] != subfieldDelim {
			next := indexByte(rest, subfieldDelim)
			if next < 0 {
				break
			}
			rest = rest[next:]
			continue
		}
		if len(rest) < 2 {
			break
		}
		code := rest[1]
		rest = rest[2:]
		end := indexByte(rest, subfieldDelim)
		var val []byte
		if end < 0 {
			val, rest = rest, nil
		} else {
			val, rest = rest[:end], rest[end:]
		}
		f.Subfields = append(f.Subfields, Subfield{Code: code, Value: string(val)})
	}
	return f
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}
