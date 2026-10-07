package ogg

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestReadValidPage(t *testing.T) {
	file := buildFile(singlePacketPage(FlagBOS, 0, 7, 0, []byte("hello")))
	rd := NewReader(bytes.NewReader(file))
	p, err := rd.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if p.Offset != 0 || p.Serial != 7 || p.Sequence != 0 || !p.BOS() {
		t.Fatalf("unexpected page: %+v", p)
	}
	if string(p.Data) != "hello" {
		t.Fatalf("payload = %q", p.Data)
	}
	if _, err := rd.Next(); err != io.EOF {
		t.Fatalf("expected clean EOF, got %v", err)
	}
}

func TestBadCapturePattern(t *testing.T) {
	file := buildFile(singlePacketPage(FlagBOS, 0, 7, 0, nil))
	copy(file[0:4], []byte("OggX"))
	// Fix nothing: CRC will also fail, but capture is checked first.
	_, err := NewReader(bytes.NewReader(file)).Next()
	if err == nil || !strings.Contains(err.Error(), "offset 0") || !strings.Contains(err.Error(), "capture") {
		t.Fatalf("expected capture error with offset, got %v", err)
	}
}

func TestBadVersion(t *testing.T) {
	file := buildFile(singlePacketPage(FlagBOS, 0, 7, 0, nil))
	file[4] = 1
	_, err := NewReader(bytes.NewReader(file)).Next()
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("expected version error, got %v", err)
	}
}

func TestCRCMismatch(t *testing.T) {
	file := buildFile(pageSpec{headerType: FlagBOS, serial: 7, badCRC: true})
	_, err := NewReader(bytes.NewReader(file)).Next()
	if err == nil || !strings.Contains(err.Error(), "CRC") {
		t.Fatalf("expected CRC error, got %v", err)
	}
}

func TestTruncatedHeader(t *testing.T) {
	file := buildFile(singlePacketPage(FlagBOS, 0, 7, 0, nil))[:10]
	_, err := NewReader(bytes.NewReader(file)).Next()
	if err == nil || !strings.Contains(err.Error(), "truncated page header") {
		t.Fatalf("expected truncated header error, got %v", err)
	}
}

func TestTruncatedPayload(t *testing.T) {
	full := buildFile(singlePacketPage(FlagBOS, 0, 7, 0, []byte("hello")))
	file := full[:len(full)-2]
	_, err := NewReader(bytes.NewReader(file)).Next()
	if err == nil || !strings.Contains(err.Error(), "truncated page payload") {
		t.Fatalf("expected truncated payload error, got %v", err)
	}
}

func TestGarbageBetweenPagesIsFatal(t *testing.T) {
	good := buildFile(singlePacketPage(FlagBOS, 0, 7, 0, nil))
	file := append(append([]byte{}, good...), 0xAA, 0xBB)
	file = append(file, good...)
	rd := NewReader(bytes.NewReader(file))
	if _, err := rd.Next(); err != nil {
		t.Fatalf("first page: %v", err)
	}
	_, err := rd.Next()
	if err == nil || !strings.Contains(err.Error(), "capture") {
		t.Fatalf("expected fatal capture error at garbage, got %v", err)
	}
}
