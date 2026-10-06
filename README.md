# PLC Scan Replay (offline sequential control replayer)

An offline, deterministic replayer for a **task-defined subset of PLC
scan-cycle semantics**. It interprets how input changes drive coils,
on-delay timers and up-counters across discrete scans. It never
connects to real devices and never reads a real clock — every scan's
duration comes from the input record.

## Run

```sh
go run ./cmd/scanlogic -program examples/motor.program.json -inputs examples/motor.scans.json
```

- stdout: one JSON object per scan (JSONL) with the frozen input
  snapshot, the scan duration, per-statement before/after diffs, and
  the full resulting state.
- stderr (or `-summary file`): a human-readable change summary listing
  only the statements that actually changed their target.

```sh
go run ./cmd/scanlogic -program p.json -inputs s.json -summary summary.txt > replay.jsonl
```

## Program format

```json
{
  "inputs":   ["Start", "Stop"],
  "coils":    ["Motor"],
  "timers":   [{"name": "T1", "preset": 30}],
  "counters": [{"name": "C1", "preset": 3}],
  "statements": [
    {"coil": {"target": "Motor", "condition": {"contact": "Start"}}},
    {"ton":  {"timer": "T1", "condition": {"contact": "Motor"}}},
    {"ctu":  {"counter": "C1",
              "condition": {"contact": "Start"},
              "reset":     {"contact": "Stop"}}}
  ]
}
```

Conditions are boolean trees with exactly one operator per node:

- `{"contact": "NAME"}` — reads an external input, a coil, or the
  **done flag** of a timer/counter.
- `{"not": {...}}`, `{"and": [{...}, ...]}`, `{"or": [{...}, ...]}`.

### Validation (illegal programs are rejected before any replay)

- Every referenced name must be declared (input, coil, timer, counter).
- Each coil/timer/counter may be written by **at most one** statement.
- Statement kind must match the declared kind of its target.
- Presets must be positive integers; exactly one statement kind per
  statement; exactly one operator per condition node.
- Input records must give a **complete** input snapshot (all declared
  inputs, no unknown ones) and a **positive integer** `duration_ms`.

## Scan semantics (the defined subset)

1. The external input snapshot is frozen at the start of the scan.
2. Statements execute **strictly in order**, one after another:
   - a write from an earlier statement is immediately visible to later
     statements in the same scan;
   - a statement that reads a state written by a *later* statement sees
     the **old** value. Statements are never evaluated in parallel.
3. Initial state: all booleans false, all accumulators/counts zero.

### TON (non-retentive on-delay timer)

- Condition true: adds this scan's `duration_ms` to the accumulator;
  when it reaches the preset the accumulator is **capped** at the
  preset and `done` is set.
- Condition false: accumulator and `done` clear immediately.

### CTU (up counter)

- Counts **only** on a false→true edge of the condition; holding the
  condition true does not count again.
- Count is capped at the preset; `done` is set when the count reaches
  the preset and persists until reset.
- **Reset has priority** over counting. The edge memory is updated
  every scan even while reset is held, so releasing reset while the
  condition is still true does **not** produce an extra count.

## Tests

```sh
go test ./...
```

Covers same-scan statement ordering (forward visibility / old-value
reads), self-holding circuits, TON accumulation/cap/clear, CTU edge
detection, preset capping, reset priority and the reset-release edge
boundary, plus rejection of illegal programs and input logs.

## Examples

- `examples/motor.program.json` — seal-in motor circuit with a run-time
  limit timer and a parts counter; either one ends the batch.
- `examples/motor.scans.json` — eight recorded scans exercising start,
  timer expiry, counting to preset, and reset.
