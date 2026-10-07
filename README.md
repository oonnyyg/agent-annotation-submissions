# DNS Wire Workbench

An offline DNS message workbench built on the Go standard library. It decodes
raw binary DNS messages (e.g. extracted from packet captures) into JSON for
inspecting name compression and record boundaries, and encodes supported JSON
documents back into wire format. It never sends network traffic.

## Usage

```
go build ./cmd/dnsworkbench

dnsworkbench decode [-in packet.bin]  [-out packet.json]
dnsworkbench encode [-in packet.json] [-out packet.bin]
```

`-in` / `-out` default to stdin / stdout. Output is only written after the
whole message is converted successfully, so a failed run never truncates or
overwrites an existing output file.

Errors are reported as structured JSON on stderr with a non-zero exit code:

```json
{"offset":12,"reason":"compression pointer loop at offset 12"}
```

`offset` is the byte position in the wire message (decode) or 0 for
document-level encode errors.

## Supported scope

- RFC 1035 header plus all four sections (question, answer, authority,
  additional). Section counts and rdata lengths are derived from the actual
  data when encoding; the message must end exactly after the declared
  sections when decoding.
- IN-class records: `A`, `CNAME`, `MX`, `TXT`. Record order and label case
  are preserved.
- Unknown types or non-IN classes decode as raw hex rdata and are **refused**
  on encode.
- Domain names are ASCII label sequences; the root name is `[]`.
- Not implemented: TCP length framing, DNSSEC, EDNS, non-ASCII labels.

## JSON representation

```json
{
  "header": {"id": 4660, "qr": true, "opcode": 0, "aa": true, "tc": false,
             "rd": true, "ra": true, "z": 0, "rcode": 0},
  "questions": [{"name": ["WWW", "Example", "COM"], "type": "A", "class": "IN"}],
  "answers": [
    {"name": ["WWW", "Example", "COM"], "type": "A", "class": "IN",
     "ttl": 300, "rdata": {"a": "192.0.2.1"}},
    {"name": ["Example", "COM"], "type": "MX", "class": "IN", "ttl": 60,
     "rdata": {"mx": {"preference": 10, "exchange": ["mail", "Example", "COM"]}}},
    {"name": ["Example", "COM"], "type": "TXT", "class": "IN", "ttl": 60,
     "rdata": {"txt": ["v=spf1 -all", "", "hex:00ff"]}}
  ],
  "authorities": [],
  "additionals": []
}
```

- `name` is an array of labels; `[]` is the root. Case is preserved exactly.
- `rdata` variants: `{"a": "192.0.2.1"}`, `{"cname": ["host", "example"]}`,
  `{"mx": {"preference": 10, "exchange": [...]}}`, `{"txt": ["...", ...]}`.
- TXT strings keep their boundaries: multiple strings and empty strings are
  never merged. Non-UTF-8 bytes are rendered as `"hex:<hex>"`.
- Unknown records decode as `{"type": "TYPE99", "rdata": {"hex": "..."}}`.

## Name compression handling

- Decoding distinguishes the bytes a name occupies at its own position from
  its expanded length, follows multi-level pointers, and rejects truncated
  names, pointer loops, pointers into the header or past the message end,
  reserved label types, labels over 63 bytes, and expanded names over 255
  bytes.
- Encoding reuses previously encoded suffixes with compression pointers.
  Pointers are only emitted when the target offset fits in 14 bits
  (<= 0x3FFF); otherwise plain labels are written, so no illegal reference
  is ever produced.

## Development

```
go test ./...
```

Tests cover compressed and uncompressed round trips, multi-level pointers,
TXT boundary preservation, and malicious inputs (truncation, pointer loops,
out-of-bounds pointers, rdata crossing its declared boundary, trailing
bytes).
