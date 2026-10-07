package copybook

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// BinToJSONL reads fixed-length records and writes one JSON object per line.
// A trailing partial record is an error. Nothing is written to w unless the
// whole input converts cleanly.
func BinToJSONL(l *Layout, r io.Reader, w io.Writer) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if len(data)%l.Length != 0 {
		return fmt.Errorf("input is %d bytes: not a multiple of record length %d (truncated record)",
			len(data), l.Length)
	}
	var buf bytes.Buffer
	for off, recNo := 0, 1; off < len(data); off, recNo = off+l.Length, recNo+1 {
		obj, err := l.DecodeRecord(data[off:off+l.Length], recNo)
		if err != nil {
			return err
		}
		line, err := json.Marshal(obj)
		if err != nil {
			return err
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	_, err = w.Write(buf.Bytes())
	return err
}

// JSONLToBin reads JSON lines and writes concatenated fixed-length records.
// Nothing is written to w unless every line converts cleanly.
func JSONLToBin(l *Layout, r io.Reader, w io.Writer) error {
	var buf bytes.Buffer
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	recNo := 0
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		recNo++
		var obj map[string]any
		dec := json.NewDecoder(bytes.NewReader(line))
		if err := dec.Decode(&obj); err != nil {
			return &RecordError{recNo, l.Root.Name, "invalid JSON: " + err.Error()}
		}
		rec, err := l.EncodeRecord(obj, recNo)
		if err != nil {
			return err
		}
		buf.Write(rec)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}
