# Ellipsoidal Route Sampler

Offline command line tool (Python standard library only) that converts
ordered latitude/longitude waypoints into ellipsoidal segment distances
and equal-spacing samples. Every step is deterministic and reproducible.

## Features

- Vincenty inverse solution for per-segment distance and initial
  bearing; Vincenty direct solution for in-segment sample positions.
- WGS84 by default; custom ellipsoid via positive semi-major axis and
  non-negative flattening below 0.01. Angles in degrees, lengths in
  metres, no height.
- Antimeridian crossing supported. Longitudes normalised to
  `[-180, 180)`, bearings to `[0, 360)`.
- Equal-spacing sampling over cumulative route distance; the step
  remainder carries across segments. The final waypoint is appended
  once (never duplicated). Coincident waypoints add no mileage, a
  zero-length segment has an undefined (empty) bearing, and a fully
  coincident route yields exactly one sample.
- Explicit iteration tolerance and iteration cap. Non-convergent
  (near-antipodal) segments are reported as failed with the failing
  segment index — no spherical fallback, no fake totals or samples.
  Other routes are processed independently.
- Pole waypoints, non-finite values and invalid parameters are
  rejected with the route id and CSV row number.
- Outputs `segments.csv`, `samples.csv` and `diagnostics.json`.
  Samples keep cumulative mileage and owning segment; a sample landing
  exactly on a shared waypoint belongs to the segment starting there.

## Input formats

`routes.csv` (header required, rows of one route keep file order):

```csv
route_id,lat,lon
demo,31.2304,121.4737
demo,35.6762,139.6503
```

`settings.json` (all keys optional except `sample_interval`):

```json
{
  "ellipsoid": {"semi_major_axis": 6378137.0,
                "flattening": 0.0033528106647474805},
  "sample_interval": 50000.0,
  "tolerance": 1e-12,
  "max_iterations": 200
}
```

## Running

```sh
python -m ellipsoidal_route_sampler \
    --routes routes.csv --settings settings.json --out-dir out
```

(with `PYTHONPATH=src`, or `pip install .` and use `route-sampler`).

Exit codes: `0` all routes ok, `1` at least one route failed
(diagnostics still written), `2` invalid input.

## Testing

```sh
python -m unittest discover -s tests
```

Covers inverse/direct round-trip consistency, antimeridian crossing,
cross-segment sampling with remainder carry-over, boundary segment
ownership, degenerate routes, non-convergence reporting and input
validation.

## Accuracy and limitations

- Vincenty's series are accurate to well under a millimetre for most
  geodesics, but the iteration can fail for near-antipodal points;
  such segments are reported as failures instead of being
  approximated.
- Double precision and the series truncation limit absolute accuracy
  to roughly 0.1 mm over very long routes.
- Pole waypoints (|lat| = 90) are rejected; bearings are undefined
  (empty) for zero-length segments.
- Height is ignored; distances are on the reference ellipsoid only.
