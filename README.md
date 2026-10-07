# Deficit Round-Robin Replay

A pure-Go (standard library only) simulator that replays packet arrivals
from CSV over a single output link scheduled by deficit round-robin (DRR).
No real network is involved; everything is discrete-event simulation with
exact rational time arithmetic (`math/big.Rat`).

## Usage

```sh
go run . -config examples/config.json -events examples/arrivals.csv
go test ./...
```

## Input formats

Config (JSON):

```json
{
  "linkRateBytesPerSec": 1000,
  "streams": [
    { "id": "alpha", "quantum": 100, "maxWaitingBytes": 300 }
  ]
}
```

- `linkRateBytesPerSec` — positive link rate; transmission of a packet of
  `L` bytes takes `L / rate` seconds and is non-preemptive.
- `quantum` — positive byte credit added per visit to a non-empty queue.
- `maxWaitingBytes` — positive per-stream waiting-room capacity in bytes.

Arrivals (CSV, header required): `time,stream,packet,length`

- `time` — non-negative decimal seconds (exact rational parse).
- `packet` — globally unique id; `length` — positive integer bytes.
- Events are sorted by time with a stable sort: arrivals at the same
  instant keep their input order.

## Event ordering and deficit rules

- Streams are visited cyclically in lexicographic id order. Visiting a
  non-empty queue adds the quantum to that stream's deficit **once**;
  idle streams receive nothing and never accumulate credit.
- While the head-of-queue packet fits in the current deficit, it is sent
  immediately (deficit decreases by its length). If the head is too large,
  the balance is kept and the visit moves to the next stream — smaller
  packets behind the head are never skipped to. Oversized heads are
  therefore sent after enough rounds of accumulation; this is normal
  progress, not a stall.
- A transmission finishing at time `t` first admits all arrivals with
  `time <= t` (in stable order), then the current visit continues without
  adding the quantum again. A packet already in transmission does not
  count against the waiting-room capacity.
- Only when the queue is empty after a send (and same-instant admissions)
  does the visit end and the deficit reset to zero — leftover balance is
  not carried into the next burst.
- When every stream is idle, the clock jumps to the next arrival and the
  round restarts from the smallest stream id. Round-robin traversal itself
  consumes no simulated time.
- An arrival that would push a stream's waiting bytes over
  `maxWaitingBytes` is dropped (only the new packet) and recorded with a
  reason; the existing queue is untouched. After the input ends, all
  admitted queues are drained.

## Output

The CLI prints three sections:

- `timeline` — every `SEND` (stream, packet, length, arrive/start/finish,
  wait) in transmission order, then every `DROP` with its reason.
- `per-stream stats` — admitted/sent/dropped counts and bytes, average and
  maximum waiting time (send start minus arrival) per stream.
- `reconciliation` — byte books: `arrived = admitted + dropped` and, since
  queues are drained, `admitted = sent`; `balanced=true` confirms it.

## Layout

- `model.go` — config, packet, record and stats types.
- `parse.go` — JSON config and CSV arrival loading/validation.
- `sim.go` — the DRR discrete-event simulator.
- `main.go` — CLI and report formatting.
- `sim_test.go` — tests: unequal lengths with head-of-line blocking,
  idle recovery with multi-round accumulation, boundary arrivals at
  completion instants, stable same-instant ordering, capacity drops,
  in-flight capacity exclusion, and deficit reset after drain.
- `examples/` — sample config and arrival trace.
