package marc

import "io"

// BadRecord describes a record that failed structural validation.
type BadRecord struct {
	Offset int
	Length int // bytes skipped; 0 when parsing stopped
	Err    string
}

// ReadAll reads consecutive MARC21 records from r. Records that fail
// validation are isolated using the leader length when it is a plausible
// integer (>= MinRecordLength and within the remaining bytes); otherwise
// reading stops and the position is reported via stopErr.
func ReadAll(r io.Reader) (records []*Record, bad []BadRecord, stopErr error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}
	pos := 0
	for pos < len(data) {
		recLen, ok := leaderLength(data[pos:])
		if !ok {
			return records, bad, &ParseError{Offset: pos, Msg: "invalid leader length; cannot isolate record, stopping"}
		}
		rec, err := ParseRecord(data[pos:pos+recLen], pos)
		if err != nil {
			bad = append(bad, BadRecord{Offset: pos, Length: recLen, Err: err.Error()})
			pos += recLen
			continue
		}
		records = append(records, rec)
		pos += recLen
	}
	return records, bad, nil
}

// leaderLength reads the 5-digit record length. It is plausible when the
// remaining data has at least a full leader, the length parses as an
// integer, is at least MinRecordLength, and does not exceed the remaining
// bytes.
func leaderLength(data []byte) (int, bool) {
	if len(data) < LeaderSize {
		return 0, false
	}
	n, ok := parseDecimal(data[0:5])
	if !ok || n < MinRecordLength || n > len(data) {
		return 0, false
	}
	return n, true
}
