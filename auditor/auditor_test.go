package auditor

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func TestPipelinedRequests(t *testing.T) {
	r1 := "GET /a HTTP/1.1\r\nHost: x\r\n\r\n"
	body := "hello"
	r2 := "POST /b HTTP/1.1\r\nHost: y\r\nContent-Length: 5\r\n\r\n" + body
	rep := Audit([]byte(r1 + r2))
	if rep.Error != nil {
		t.Fatalf("unexpected error: %v", rep.Error)
	}
	if len(rep.Requests) != 2 {
		t.Fatalf("want 2 requests, got %d", len(rep.Requests))
	}
	if rep.Requests[0].StartOffset != 0 || rep.Requests[0].EndOffset != int64(len(r1)) {
		t.Fatalf("bad offsets r1: %+v", rep.Requests[0])
	}
	if rep.Requests[1].StartOffset != int64(len(r1)) ||
		rep.Requests[1].EndOffset != int64(len(r1)+len(r2)) {
		t.Fatalf("bad offsets r2: %+v", rep.Requests[1])
	}
	if rep.Requests[1].BodyLength != 5 || rep.Requests[1].BodySHA256 != sha([]byte(body)) {
		t.Fatalf("bad body digest: %+v", rep.Requests[1])
	}
}

func TestBodyContainingRequestLikeText(t *testing.T) {
	body := "GET /evil HTTP/1.1\r\nHost: z\r\n\r\n"
	r := "POST /up HTTP/1.1\r\nHost: x\r\nContent-Length: " +
		"30" + "\r\n\r\n" + body[:30]
	rep := Audit([]byte(r))
	if rep.Error != nil {
		t.Fatalf("unexpected error: %v", rep.Error)
	}
	if len(rep.Requests) != 1 {
		t.Fatalf("body text must not start a new request: %+v", rep.Requests)
	}
}

func TestTruncatedBody(t *testing.T) {
	r := "POST /a HTTP/1.1\r\nHost: x\r\nContent-Length: 10\r\n\r\nabc"
	rep := Audit([]byte(r))
	if rep.Error == nil {
		t.Fatal("expected truncation error")
	}
	if !strings.Contains(rep.Error.Message, "truncated body") {
		t.Fatalf("unexpected message: %v", rep.Error)
	}
	if len(rep.Requests) != 0 {
		t.Fatalf("no complete request expected, got %d", len(rep.Requests))
	}
}

func TestErrorKeepsPriorRequests(t *testing.T) {
	good := "GET /a HTTP/1.1\r\nHost: x\r\n\r\n"
	bad := "GET /b HTTP/1.1\r\nHost: x\r\nContent-Length: 5\r\nContent-Length: 5\r\n\r\n"
	rep := Audit([]byte(good + bad))
	if rep.Error == nil {
		t.Fatal("expected duplicate Content-Length error")
	}
	if len(rep.Requests) != 1 {
		t.Fatalf("prior request must be kept, got %d", len(rep.Requests))
	}
}

func TestConflictingHeaders(t *testing.T) {
	cases := []string{
		"GET / HTTP/1.1\r\nHost: x\r\nContent-Length: 1\r\nContent-Length: 1\r\n\r\n",
		"GET / HTTP/1.1\r\nHost: x\r\nContent-Length: 1, 1\r\n\r\n",
		"GET / HTTP/1.1\r\nHost: x\r\nContent-Length: 4\r\nTransfer-Encoding: chunked\r\n\r\n",
		"GET / HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: gzip\r\n\r\n",
		"GET / HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked, gzip\r\n\r\n",
		"GET / HTTP/1.1\r\nHost: x\r\nContent-Length: -1\r\n\r\n",
		"GET / HTTP/1.1\r\nHost: x\r\nContent-Length: 0x10\r\n\r\n",
	}
	for i, c := range cases {
		rep := Audit([]byte(c))
		if rep.Error == nil {
			t.Fatalf("case %d: expected error", i)
		}
	}
}

func TestChunkedBody(t *testing.T) {
	r := "POST /c HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n" +
		"4\r\nWiki\r\n5\r\npedia\r\n0\r\nX-Meta: ok\r\n\r\n" +
		"GET /next HTTP/1.1\r\nHost: y\r\n\r\n"
	rep := Audit([]byte(r))
	if rep.Error != nil {
		t.Fatalf("unexpected error: %v", rep.Error)
	}
	if len(rep.Requests) != 2 {
		t.Fatalf("want 2 requests, got %d", len(rep.Requests))
	}
	if rep.Requests[0].BodyLength != 9 || rep.Requests[0].BodySHA256 != sha([]byte("Wikipedia")) {
		t.Fatalf("bad chunked digest: %+v", rep.Requests[0])
	}
}

func TestChunkedRejections(t *testing.T) {
	cases := []string{
		"POST / HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n4;ext=1\r\nWiki\r\n0\r\n\r\n", // extension
		"POST / HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n4\r\nWikiXX0\r\n\r\n",         // bad CRLF after data
		"POST / HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n0\r\nHost: y\r\n\r\n",         // Host trailer
		"POST / HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n0\r\nContent-Length: 1\r\n\r\n",
		"POST / HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\nzz\r\n",  // bad size
		"POST / HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n4\r\nWi", // truncated chunk
	}
	for i, c := range cases {
		rep := Audit([]byte(c))
		if rep.Error == nil {
			t.Fatalf("case %d: expected error", i)
		}
	}
}

func TestStrictSyntax(t *testing.T) {
	cases := []string{
		"GET / HTTP/1.1\nHost: x\r\n\r\n",              // bare LF
		"GET / HTTP/1.1\r\nHost : x\r\n\r\n",           // space before colon
		"GET / HTTP/1.1\r\nHost: x\r\n folded\r\n\r\n", // folded header
		"GE T / HTTP/1.1\r\nHost: x\r\n\r\n",           // bad method
		"GET http://x/ HTTP/1.1\r\nHost: x\r\n\r\n",    // target not slash
		"GET / HTTP/1.0\r\nHost: x\r\n\r\n",            // wrong version
		"GET / HTTP/1.1\r\n\r\n",                       // missing Host
		"GET / HTTP/1.1\r\nHost: x\r\nHost: y\r\n\r\n", // two Hosts
		"GET / HTTP/1.1\r\nHost:\r\n\r\n",              // empty Host
	}
	for i, c := range cases {
		rep := Audit([]byte(c))
		if rep.Error == nil {
			t.Fatalf("case %d: expected error", i)
		}
	}
}

func TestNoFramingHeadersMeansEmptyBody(t *testing.T) {
	r := "GET / HTTP/1.1\r\nHost: x\r\n\r\nGET /2 HTTP/1.1\r\nHost: y\r\n\r\n"
	rep := Audit([]byte(r))
	if rep.Error != nil {
		t.Fatalf("unexpected error: %v", rep.Error)
	}
	if len(rep.Requests) != 2 || rep.Requests[0].BodyLength != 0 {
		t.Fatalf("bad result: %+v", rep.Requests)
	}
}
