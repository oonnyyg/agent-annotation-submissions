# ntp-exchange-auditor

Offline NTP time-synchronization auditor. It reads recorded NTPv4
request/response exchanges from a JSONL file, reconstructs clock offset
samples, separates bad samples from usable ones, and reports a per-peer
summary. It never sends packets and never modifies the system clock.

## Usage

```
go run ./cmd/ntpaudit [-window N] [records.jsonl]
```

Reads stdin when no file is given. `-window` (default 8) is the number of
most recent valid samples kept per peer.

## Input format

One JSON object per line:

```json
{"peer":"ntp1","request_hex":"2300...","response_hex":"2402...",
 "sent_local":"2024-01-01T00:00:00Z","received_local":"2024-01-01T00:00:00.025Z",
 "approx_utc":"2024-01-01T00:00:00Z"}
```

- `peer`: peer identifier used for grouping.
- `request_hex` / `response_hex`: hex-encoded 48-byte NTPv4 packets.
- `sent_local` / `received_local`: local-clock times when the request was
  sent (T1) and the response received (T4), RFC 3339.
- `approx_utc`: approximate UTC base used to resolve the era of the 32-bit
  NTP seconds in the response.

## Time representation

NTP timestamps are 32-bit era-relative seconds plus a 32-bit fraction. The
seconds field wraps every 2^32 seconds (~136 years, next wrap 2036-02-07),
so an absolute time is recovered by choosing the era nearest to
`approx_utc`. If the base sits exactly half an era (2^31 seconds) from the
candidate, both eras are equally near and the record is rejected as
ambiguous. All expansion uses exact integer arithmetic — the 32-bit
fraction is converted to nanoseconds without floating point — so
timestamps across the 2036 wrap keep their correct order and small
fraction differences are never collapsed by rounding a large float.

## Acceptance rules

A record is accepted only if all of the following hold:

- Both packets are exactly 48 bytes (no extension fields), version 4.
- The request is mode 3 (client); the response is mode 4 (server).
- Response stratum is 1-15. Stratum 0 is a Kiss-o'-Death control message
  and is reported separately with its 4-byte ASCII code (e.g. `RATE`).
- Response leap indicator is not 3 (server unsynchronized).
- The response origin timestamp equals the request transmit timestamp.
- The server transmit timestamp (T3) is not earlier than its receive
  timestamp (T2).
- The computed round-trip delay is non-negative.
- The request has not already produced a valid sample; only the first
  valid response to a given request is recorded, later ones are reported
  as duplicates.

Rejected and duplicate records never displace valid samples in the window.

## Offset, delay, filtering

From the four timestamps (T1/T4 local clock, T2/T3 server clock expanded
against `approx_utc`):

```
offset = ((T2 - T1) + (T3 - T4)) / 2
delay  = (T4 - T1) - (T3 - T2)
```

Each peer keeps the most recent `-window` valid samples. The current
offset estimate is the window sample with the smallest delay; ties go to
the most recent sample. Jitter is the root-mean-square of the other window
samples' offsets relative to the selected offset; a single-sample window
has zero jitter.

## Output

Per line: `accepted` with offset/delay and expanded server timestamps, or
`rejected` / `duplicate` with the reason. After all records, a per-peer
summary lists the selected offset, jitter, and the window contents with
the selected sample marked `*`.

See `testdata/sample.jsonl` for a runnable example.
