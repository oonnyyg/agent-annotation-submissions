# HTTP/1.1 Request Boundary Auditor

An **offline** auditor that scans a local binary file as a strict HTTP/1.1
request stream and reports the exact byte boundaries of every request. It
exists to surface gateway/backend framing disagreements (request smuggling
class bugs). It **does not** listen on ports, forward traffic, or replace a
real HTTP server.

## Usage

```
go build -o auditor .
./auditor capture.bin          # human-readable report
./auditor -json capture.bin    # JSON report on stdout
```

Exit code is `0` when the whole file parses cleanly, `1` when a framing
error is found, `2` for usage/IO errors.

## Strict subset accepted

This tool intentionally accepts only a strict subset of HTTP/1.1:

- Request line: `METHOD SP /target SP HTTP/1.1 CRLF`. The method must be a
  valid token; the target must start with `/` and contain no whitespace;
  only `HTTP/1.1` is accepted.
- Header lines must end with CRLF. Obs-fold (folded/continued headers) and
  whitespace before the colon are rejected. Field names are tokens and are
  matched case-insensitively.
- Exactly one non-empty `Host` header per request.
- `Content-Length`: a single non-negative decimal integer. Duplicates and
  comma lists are rejected even when the values agree. It must not appear
  together with `Transfer-Encoding`.
- `Transfer-Encoding`: only a single `chunked` value is accepted.
- With neither header the body length is zero. With `Content-Length` exactly
  that many bytes are read; request-line-looking bytes inside the body are
  never re-interpreted.
- Chunked bodies: hexadecimal chunk sizes, no chunk extensions, a CRLF is
  required after every chunk, and a terminating zero chunk. Trailer fields
  are allowed but must not contain `Content-Length`, `Transfer-Encoding`,
  or `Host`. The next request starts only after the blank line ending the
  trailers.

## Report

For each complete request the report contains the raw `[startOffset,
endOffset)` byte range, the decoded body length, and the body SHA-256.
Chunk-size lines, chunk CRLFs and trailer fields are framing bytes and are
**not** included in the digest.

On the first framing violation the auditor reports the raw byte offset of
the error, keeps all previously completed requests, and stops — it never
scans forward guessing a new request start. `trailingBytes` counts bytes
after the last complete request.

## Limitations

This is an audit tool for a strict subset, not a full HTTP/1.1
implementation: no absolute-form/authority-form/asterisk-form targets, no
chunk extensions, no obs-fold, no responses, no connection semantics. Do
not use it as a substitute for a real HTTP server or proxy.
