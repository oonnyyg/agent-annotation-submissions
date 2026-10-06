# histogram-window-audit

Offline tool (Go standard library only) that turns cumulative latency
histogram snapshots from multiple service instances into explainable
window statistics. It never connects to a monitoring service; it reads
a JSONL snapshot file.

## Input format

One JSON object per line, order irrelevant:

```json
{"instance":"a","ts":20,"boot":"a2","bounds":[0.1,0.5,1],"counts":[3,6,8,9],"sum":2.4}
```

- `instance`, `ts` (integer seconds) identify the snapshot; `boot` is a
  boot/generation id that changes when the process restarts.
- `bounds` are the finite bucket upper bounds: strictly increasing,
  positive, at least one required. The final bucket is implicitly
  `(last bound, +Inf]`.
- `counts` are cumulative bucket counters (`len == len(bounds)+1`),
  non-decreasing; the last entry is the total request count.
- `sum` is the cumulative total latency; it must be `0` when the total
  count is `0`.
- Duplicate `(instance, ts)` pairs are rejected.

## Usage

```
go run . -input snapshots.jsonl -start 10 -end 20 \
    -quantiles 0,0.5,0.9,0.99 -csv buckets.csv
```

Prints a JSON report to stdout: observed request count, average latency
(`null` when zero requests were observed), requests per second, the
requested quantiles, and any restart gaps. `-csv` writes a per-bucket
segment table (`bucket_lower,bucket_upper,count`).

## Semantics

- **Window endpoints**: both `-start` and `-end` must have exact
  snapshots for *every* instance; bucket bounds must be identical
  across the whole window and across instances. Missing endpoints or
  mismatched bounds are errors — the tool never interpolates missing
  snapshots and never silently drops an instance.
- **Per-instance first**: all adjacent snapshot intervals are computed
  per instance, then bucket deltas and sums are merged. Instances are
  never summed first and checked for restarts afterwards.
- **Same boot**: an interval is measured by subtraction. Negative
  bucket deltas or a negative sum delta without a boot change are
  rejected as corrupt input.
- **Boot change**: the interval counts only the new snapshot's
  cumulative values, and a `restart_gaps` entry is emitted.

## Known gaps and assumptions

- **Restart gap**: when the boot id changes, requests served by the old
  process after its last snapshot are unobservable. The report flags
  these intervals in `restart_gaps` but does **not** claim to
  reconstruct traffic during the restart.
- **Quantile interpolation**: a quantile is located by cumulative
  count, then linearly interpolated within its finite bucket (the first
  bucket's lower bound is taken as 0). A quantile landing in the `+Inf`
  bucket returns the last finite upper bound. Quantile `0` of a
  non-empty distribution is `0`. With zero observed requests, averages
  and quantiles are `null`.
- **Requests per second** divides observed requests by the full window
  length, including any restart gaps.

## Tests

```
go test ./...
```

Covers unordered input, restart handling, `+Inf` bucket quantiles,
missing endpoints, inconsistent bounds, and corrupt counters.
