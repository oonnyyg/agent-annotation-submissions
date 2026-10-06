# http-framing-auditor

Offline HTTP/1.1 request boundary auditor. It reads a local binary file and
reports where each request starts and ends, plus the decoded body length and
SHA-256 digest. It never listens on ports and never forwards traffic; it is a
static analysis tool for captures where a gateway and a backend might disagree
on request boundaries (request smuggling class of bugs).

## Usage

```
go run ./cmd/http-framing-auditor <file>
```

Prints a JSON report to stdout. Exit code is 0 when the whole file parses
cleanly, 1 when an audit error was found, 2 on usage/IO errors.

Report fields per request: `index`, `start_offset`, `end_offset` (raw byte
offsets into the file), `method`, `target`, `body_length` (decoded length),
`body_sha256` (digest of the decoded body only; chunk framing bytes and
trailer fields are excluded). On the first error the report contains all
requests completed so far plus an `error` object with the raw `offset` and a
message. The auditor never tries to guess a new resynchronization point after
a failure.

## Strict subset accepted

This tool intentionally accepts only a strict subset of HTTP/1.1. Anything
outside it is an error, not a best-effort parse:

- Request line: `METHOD SP /target SP HTTP/1.1` terminated by CRLF. Method
  must be a valid token; target must start with `/` and contain no
  whitespace; only `HTTP/1.1` is accepted.
- Header lines must end with CRLF (bare CR or LF rejected). Obs-fold
  (folded/continued headers) and whitespace before the colon are rejected.
  Header names are matched case-insensitively.
- Exactly one non-empty `Host` header per request.
- `Content-Length`: a single non-negative decimal integer. Duplicates and
  comma-separated lists are rejected even when the values agree.
  `Content-Length` together with `Transfer-Encoding` is rejected.
- `Transfer-Encoding`: only the single value `chunked`.
- With neither framing header the body length is zero; the next byte after
  the header blank line starts the next request. With `Content-Length` the
  body is exactly that many bytes, even if it contains request-like text.
- Chunked bodies: hexadecimal chunk sizes, no chunk extensions, each chunk
  data followed by CRLF, terminated by a zero-size chunk. Trailer fields are
  allowed except `Content-Length`, `Transfer-Encoding`, and `Host`. The next
  request starts only after the blank line ending the trailer section.

## Scope

This is an auditor for a strict subset of HTTP/1.1 framing. It is **not** a
replacement for a complete HTTP server or parser: it does not implement
semantics, routing, absolute-form targets, connection handling, or the full
RFC 9112 grammar, and it must not be used to make allow/deny decisions in a
live path.

## Tests

```
go test ./...
```

Covers pipelined (coalesced) requests, bodies containing request-like text,
truncated bodies, duplicate/conflicting framing headers, chunked decoding,
trailer restrictions, and strict syntax rejections.
