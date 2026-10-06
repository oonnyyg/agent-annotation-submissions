
# Bitemporal Asset Register

Local, database-free bitemporal ownership archive for equipment handovers,
implemented with the Go standard library only.

## The two times

- **Business time** (`from`/`to`, calendar dates `YYYY-MM-DD`): when a device
  *actually* belonged to a unit in the real world. Spans are half-open
  `[from, to)` and must be non-empty.
- **Record time** (`recorded_at`, integer seconds): when the system *learned*
  a fact. It is always supplied by the caller — the register never reads the
  local clock. Successful batches must have strictly increasing `recorded_at`.

Because the two are independent, a handover can be recorded late
(back-dated business span) and audits can ask both "who owned it on day X?"
and "what did we know as of record time T?". A late back-dated revision
changes answers for cutoffs at or after its `recorded_at`, but never for
earlier cutoffs.

## JSONL batches

One batch per line:

```json
{"recorded_at":1000,"revisions":[{"device":"pump-7","unit":"workshop-a","from":"2026-01-01","to":"2026-06-01"}]}
```

- Empty `unit` revokes ownership for the span.
- Spans of the same device within one batch must not overlap.
- Each batch is validated as a whole before applying: a failed batch changes
  nothing and does not advance the last record time. A failed submit file
  leaves the on-disk archive untouched.
- Revisions override only the business span they cover; uncovered ends of
  older spans are preserved, and gaps with no prior ownership can be filled.
  Adjacent spans with the same unit merge in query output; the underlying
  revision history is never rewritten.

## Commands

```
register submit   <archive> <batches.jsonl>
register query    <archive> <device> <YYYY-MM-DD> <cutoff>   # owner, or NO RECORD
register timeline <archive> <device> <cutoff>                # full business timeline (JSON)
register diff     <archive> <device> <cutoffA> <cutoffB>     # minimal changed segments (JSON)
```

`query` and `timeline` answer using only revisions with
`recorded_at <= cutoff`. `NO RECORD` (empty unit in JSON output) is distinct
from a known owner. `diff` compares the same device's full business timeline
under two knowledge cutoffs and emits the minimal merged segments where the
owner actually changed, with `before`/`after` values (empty = no record).

## Archive

The archive is a single JSON file containing every applied batch plus a
SHA-256 checksum. Reloading replays the batches, so historical queries and
diffs are identical after restart. Any corruption, tampering, or invalid
batch sequence makes loading fail — a corrupt archive is never used.

## Tests

```
go test ./...
```

Covers cross-span overrides, retroactive revocation, all-or-nothing batch
failure, diff merging, and archive round-trip/corruption.
