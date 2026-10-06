package marc

import (
	"fmt"
	"unicode/utf8"
)

const (
	FieldTerminator    = 0x1E
	RecordTerminator   = 0x1D
	SubfieldDelimiter  = 0x1F
	LeaderSize         = 24
	DirectoryEntrySize = 12
	MinRecordLength    = 26
)

// Subfield is one MARC subfield; Code is the byte after 0x1F.
type Subfield struct {
	Code byte
	Data string
}

// Field is one MARC field. Control fields (001-009) keep the raw
// value in Value and have no subfields.
type Field struct {
	Tag       string
	Value     string
	Subfields []Subfield
}

// Record is a parsed MARC21 record.
type Record struct {
	Leader string
	Fields []Field
}

// ParseError describes a structural problem at a byte offset in the input.
type ParseError struct {
	Offset int
	Msg    string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("offset %d: %s", e.Offset, e.Msg)
}

func parseDecimal(b []byte) (int, bool) {
	if len(b) == 0 {
		return 0, false
	}
	n := 0
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

// ParseRecord parses one record from data, which must be exactly the
// record bytes (length taken from the leader).
func ParseRecord(data []byte, baseOffset int) (*Record, error) {
	if len(data) < MinRecordLength {
		return nil, &ParseError{Offset: baseOffset, Msg: fmt.Sprintf("record too short: %d bytes", len(data))}
	}
	if !utf8.Valid(data) {
		return nil, &ParseError{Offset: baseOffset, Msg: "record is not valid UTF-8"}
	}
	recLen, ok := parseDecimal(data[0:5])
	if !ok || recLen != len(data) {
		return nil, &ParseError{Offset: baseOffset, Msg: "leader record length mismatch"}
	}
	baseAddr, ok := parseDecimal(data[12:17])
	if !ok || baseAddr < LeaderSize+1 || baseAddr > recLen {
		return nil, &ParseError{Offset: baseOffset, Msg: "invalid base address of data"}
	}
	if data[baseAddr-1] != FieldTerminator {
		return nil, &ParseError{Offset: baseOffset + baseAddr - 1, Msg: "directory not terminated by 0x1E"}
	}
	dirLen := baseAddr - LeaderSize - 1
	if dirLen%DirectoryEntrySize != 0 {
		return nil, &ParseError{Offset: baseOffset, Msg: "directory length not a multiple of 12"}
	}
	if data[recLen-1] != RecordTerminator {
		return nil, &ParseError{Offset: baseOffset + recLen - 1, Msg: "record not terminated by 0x1D"}
	}

	rec := &Record{Leader: string(data[:LeaderSize])}
	fieldAreaLen := recLen - baseAddr - 1 // exclude record terminator
	for i := 0; i < dirLen; i += DirectoryEntrySize {
		entry := data[LeaderSize+i : LeaderSize+i+DirectoryEntrySize]
		tag := string(entry[0:3])
		if !allDigits(entry[0:3]) {
			return nil, &ParseError{Offset: baseOffset + LeaderSize + i, Msg: "directory tag is not numeric"}
		}
		length, ok1 := parseDecimal(entry[3:7])
		offset, ok2 := parseDecimal(entry[7:12])
		if !ok1 || !ok2 {
			return nil, &ParseError{Offset: baseOffset + LeaderSize + i, Msg: "directory entry has non-numeric length/offset"}
		}
		if length < 1 || offset+length > fieldAreaLen {
			return nil, &ParseError{Offset: baseOffset + LeaderSize + i, Msg: fmt.Sprintf("field %s out of range (offset %d, length %d)", tag, offset, length)}
		}
		raw := data[baseAddr+offset : baseAddr+offset+length]
		if raw[length-1] != FieldTerminator {
			return nil, &ParseError{Offset: baseOffset + baseAddr + offset + length - 1, Msg: fmt.Sprintf("field %s not terminated by 0x1E", tag)}
		}
		body := raw[:length-1]
		rec.Fields = append(rec.Fields, makeField(tag, body))
	}
	return rec, nil
}

func allDigits(b []byte) bool {
	for _, c := range b {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func makeField(tag string, body []byte) Field {
	f := Field{Tag: tag}
	if tag >= "001" && tag <= "009" {
		// Control fields are not split into subfields.
		f.Value = string(body)
		return f
	}
	if len(body) >= 2 {
		body = body[2:] // indicators
	}
	for len(body) > 0 {
		if body[0] != SubfieldDelimiter || len(body) < 2 {
			// Malformed subfield structure; keep remainder as-is.
			f.Subfields = append(f.Subfields, Subfield{Code: 0, Data: string(body)})
			break
		}
		code := body[1]
		rest := body[2:]
		end := len(rest)
		for j := 0; j < len(rest); j++ {
			if rest[j] == SubfieldDelimiter {
				end = j
				break
			}
		}
		f.Subfields = append(f.Subfields, Subfield{Code: code, Data: string(rest[:end])})
		body = rest[end:]
	}
	return f
}
