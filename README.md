# Layered Refraction First-Arrival Analyser

Computes first-arrival travel times for a surface seismic line over
horizontal layers whose velocities strictly increase with depth. Pure
Python standard library; all quantities are in metres and seconds.

## Usage

```
python -m refraction input.json --csv paths.csv --json analysis.json
```

Input JSON:

```json
{
  "layers": [
    {"velocity": 500, "thickness": 10},
    {"velocity": 2000}
  ],
  "offsets": [0, 25, 50, 100],
  "range": {"min": 0, "max": 500},
  "observed": [{"offset": 100, "time": 0.089}]
}
```

- `layers`: top to bottom; the last entry is the infinite half-space and
  must omit `thickness`. Non-positive thicknesses and non-increasing or
  non-positive velocities are rejected.
- `offsets`: receiver offsets (0 is allowed).
- `range` (optional): continuous offset interval for branch-dominance
  analysis.
- `observed` (optional): picked first arrivals for residual/RMS
  reporting only; no inversion is performed.

Outputs: a CSV with every path's per-layer segments at each offset, and
a JSON file with first arrivals, tied paths, dominance intervals and
residual statistics.

## Physics

Two path families are modelled:

- **Direct wave** through the top layer: `t = x / v1`.
- **Head wave** refracted along the top of layer `k`: down and up legs
  through every overlying layer at the critical angle given by Snell's
  law (`sin(ic) = vi / vk`), plus a horizontal leg along the refractor.

### Critical distance vs. crossover distance

These are different things:

- **Critical distance** `x_c = sum 2 hi tan(ici)` is the minimum offset
  at which a refracted path physically exists. Below `x_c` the path is
  reported as *unreachable*; the linear travel-time formula is never
  extrapolated into that region.
- **Crossover distance** is where two valid travel-time branches have
  equal time and the first arrival switches from one branch to another.
  It is computed as the true intersection of the linear branches
  (bisection to 1e-10 m), restricted to offsets where both paths are
  reachable — never guessed from the sampled input offsets.

A refractor whose head wave propagates but is always overtaken by
another branch is listed under `never_first` in the dominance report.

### Tolerances

- Time ties within `1e-9 s` are all kept as joint first arrivals.
- Reachability uses a `1e-9 m` slack around the critical distance.
- Crossover locations converge to `1e-10 m`.

## Tests

```
python -m unittest discover -s tests -v
```

The suite includes analytic checks: critical distance, refracted
travel-time intercepts, direct/refracted crossover location, segment
sums, single half-space, zero offset, and a thin-layer case whose
refractor never becomes a first arrival.
