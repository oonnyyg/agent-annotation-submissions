package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func sha256hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestPipelinedRequests(t *testing.T) {
	body1 := "hello"
	body2 := "GET /fake HTTP/1.1\r\nHost: evil\r\n\r\n" // inside body, must not be parsed
	stream := "POST /a HTTP/1.1\r\nHost: x\r\nContent-Length: 5\r\n\r\n" + body1 +
		"POST /b HTTP/1.1\r\nHost: x\r\nContent-Length: 34\r\n\r\n" + body2 +
		"GET /c HTTP/1.1\r\nHost: x\r\n\r\n"
	r := Audit("test", []byte(stream))
	if r.Error != nil {
		t.Fatalf("unexpected error: %v", r.Error)
	}
	if r.RequestCount != 3 {
		t.Fatalf("want 3 requests, got %d", r.RequestCount)
	}
	if r.Requests[0].StartOffset != 0 || r.Requests[1].StartOffset != r.Requests[0].EndOffset {
		t.Fatalf("bad offsets: %+v", r.Requests)
	}
	if r.Requests[1].BodyLength != 34 || r.Requests[1].BodySHA256 != sha256hex(body2) {
		t.Fatalf("body-like text leaked into parsing: %+v", r.Requests[1])
	}
	if r.Requests[2].BodyLength != 0 || r.TrailingBytes != 0 {
		t.Fatalf("bad final request: %+v trailing=%d", r.Requests[2], r.TrailingBytes)
	}
}

func TestChunkedBody(t *testing.T) {
	stream := "POST /up HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n" +
		"4\r\nWiki\r\n5\r\npedia\r\n0\r\nX-Checksum: abc\r\n\r\n" +
		"GET /next HTTP/1.1\r\nHost: y\r\n\r\n"
	r := Audit("test", []byte(stream))
	if r.Error != nil {
		t.Fatalf("unexpected error: %v", r.Error)
	}
	if r.RequestCount != 2 {
		t.Fatalf("want 2 requests, got %d", r.RequestCount)
	}
	first := r.Requests[0]
	if first.BodyLength != 9 || first.BodySHA256 != sha256hex("Wikipedia") {
		t.Fatalf("bad decoded body: %+v", first)
	}
	if r.Requests[1].StartOffset != first.EndOffset {
		t.Fatalf("next request must start after trailer blank line")
	}
}

func TestChunkedRejections(t *testing.T) {
	cases := []struct {
		name   string
		stream string
	}{
		{"chunk extension", "POST / HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n4;ext=1\r\nWiki\r\n0\r\n\r\n"},
		{"bad chunk CRLF", "POST / HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n4\r\nWikiXX0\r\n\r\n"},
		{"trailer host", "POST / HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n0\r\nHost: y\r\n\r\n"},
		{"trailer content-length", "POST / HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n0\r\nContent-Length: 4\r\n\r\n"},
		{"truncated chunk", "POST / HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n4\r\nWi"},
	}
	for _, tc := range cases {
		r := Audit("test", []byte(tc.stream))
		if r.Error == nil {
			t.Errorf("%s: expected error", tc.name)
		}
		if r.RequestCount != 0 {
			t.Errorf("%s: request should not be recorded", tc.name)
		}
	}
}

func TestStrictSyntax(t *testing.T) {
	cases := []struct {
		name   string
		stream string
	}{
		{"lf only", "GET / HTTP/1.1\nHost: x\n\n"},
		{"folded header", "GET / HTTP/1.1\r\nHost: x\r\n X-Cont: y\r\n\r\n"},
		{"space before colon", "GET / HTTP/1.1\r\nHost : x\r\n\r\n"},
		{"bad method", "GE T / HTTP/1.1\r\nHost: x\r\n\r\n"},
		{"absolute target", "GET http://x/ HTTP/1.1\r\nHost: x\r\n\r\n"},
		{"http 1.0", "GET / HTTP/1.0\r\nHost: x\r\n\r\n"},
		{"missing host", "GET / HTTP/1.1\r\nX-A: b\r\n\r\n"},
		{"empty host", "GET / HTTP/1.1\r\nHost:\r\n\r\n"},
		{"duplicate host", "GET / HTTP/1.1\r\nHost: x\r\nHost: y\r\n\r\n"},
		{"negative length", "POST / HTTP/1.1\r\nHost: x\r\nContent-Length: -1\r\n\r\n"},
	}
	for _, tc := range cases {
		r := Audit("test", []byte(tc.stream))
		if r.Error == nil {
			t.Errorf("%s: expected error", tc.name)
		}
	}
}

func TestErrorOffsetAndKeepPrior(t *testing.T) {
	good := "GET /ok HTTP/1.1\r\nHost: x\r\n\r\n"
	bad := "GET /bad HTTP/1.1\r\nHost: x\r\nBad Header\r\n\r\n"
	r := Audit("test", []byte(good+bad))
	if r.Error == nil {
		t.Fatal("expected error")
	}
	if r.RequestCount != 1 {
		t.Fatalf("prior complete request must be kept, got %d", r.RequestCount)
	}
	if r.Error.Offset < len(good) {
		t.Fatalf("error offset %d must be inside the bad request (>= %d)", r.Error.Offset, len(good))
	}
}

func TestTruncatedBody(t *testing.T) {
	stream := "POST /a HTTP/1.1\r\nHost: x\r\nContent-Length: 10\r\n\r\nshort"
	r := Audit("test", []byte(stream))
	if r.Error == nil {
		t.Fatal("expected truncation error")
	}
	if !strings.Contains(r.Error.Message, "truncated body") {
		t.Fatalf("unexpected error: %v", r.Error)
	}
	if r.RequestCount != 0 {
		t.Fatalf("no request should be recorded, got %d", r.RequestCount)
	}
}

func TestConflictingHeaders(t *testing.T) {
	cases := []string{
		"POST / HTTP/1.1\r\nHost: x\r\nContent-Length: 3\r\nTransfer-Encoding: chunked\r\n\r\n0\r\n\r\n",
		"POST / HTTP/1.1\r\nHost: x\r\nContent-Length: 3\r\nContent-Length: 3\r\n\r\nabc",
		"POST / HTTP/1.1\r\nHost: x\r\nContent-Length: 3, 3\r\n\r\nabc",
		"POST / HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: gzip\r\n\r\n",
		"POST / HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\nTransfer-Encoding: chunked\r\n\r\n0\r\n\r\n",
	}
	for i, stream := range cases {
		r := Audit("test", []byte(stream))
		if r.Error == nil {
			t.Errorf("case %d: expected error, got none", i)
		}
		if r.RequestCount != 0 {
			t.Errorf("case %d: request should not be recorded", i)
		}
	}
}
