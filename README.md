# Zero-Dimensional Persistence

A standard-library-only Python tool that computes 0-dimensional persistent
homology (connected components) of a 2D point cloud and cross-checks the
persistence intervals against groupings at arbitrary query scales.

## Usage

```
python main.py points.csv thresholds.json --json-out report.json --csv-prefix out
```

- `points.csv`: rows of `id,x,y` (an optional `id,x,y` header line is skipped).
- `thresholds.json`: a JSON list of query thresholds, or `{"thresholds": [...]}`.
- `--json-out`: full JSON report (default `report.json`).
- `--csv-prefix`: also writes `<prefix>_intervals.csv` and `<prefix>_merges.csv`.

Run tests with `python -m unittest test_persistence`.

## Input rules

- Ids must be unique; duplicate ids are rejected.
- Coordinates must be finite numbers (`nan`/`inf` are rejected).
- Distinct ids may share identical coordinates (zero-length intervals result).
- Point count must be between 1 and 300.
- Every query threshold must be a non-negative finite number.

## Scale and representative rules

- Every point is born at scale 0. Two points are connected by an edge when
  their Euclidean distance is at most the current threshold.
- Edges are processed in increasing distance. All edges sharing the same
  distance form one batch; only 0-dimensional components are tracked (no
  higher-dimensional simplices or holes).
- When two components merge, the component whose representative id is
  lexicographically smallest survives; the other dies at that distance.
  Every merge record from a batch points directly at the representative
  after the whole batch has been applied, so results never depend on the
  input row order.
- Intervals are `[birth, death)`: at a query scale exactly equal to a merge
  distance, the merged (post-batch) state is used. Coincident points produce
  zero-length intervals `[0, 0)`, which are kept in the output.
- The final surviving component has death `"Infinity"` in the JSON report.
- For each query threshold the report lists the component members and the
  component count; the count always equals the number of intervals still
  alive at that scale (verified internally). Single-point inputs produce a
  complete report with one infinite interval.

## Files

- `persistence.py` — union-find core, batched merging, intervals, groupings.
- `io_utils.py` — CSV/JSON loading, validation, report rendering.
- `main.py` — command line interface.
- `test_persistence.py` — unit tests, including equidistant-point batches and
  shuffled-input order independence.
- `examples/` — sample input and generated report.
