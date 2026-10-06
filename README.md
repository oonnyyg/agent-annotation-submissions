# Branched Decay Inventory
 
`decay_inventory` computes time-dependent nuclide quantities, activities,
cumulative decay counts and per-branch yields for a small directed acyclic
decay network with branching and external feeds. It uses only the Python
standard library; no nuclear data files are downloaded — all half-lives,
branch probabilities and initial quantities come from the input JSON.
 
## Usage
 
```
python -m decay_inventory input.json --csv out.csv --json out.json
```
 
- CSV: one row per output time with `quantity[<name>]` and
  `activity[<name>]` columns.
- JSON: per-window cumulative decays per nuclide, per-branch yields,
  conservation residuals and the precision controls actually used.
 
On any invalid network, invalid schedule or numerical non-convergence the
program exits with status 1, prints `error: ...` to stderr and writes no
report files.
 
## Input format
 
```json
{
  "nuclides": [
    {"name": "A", "half_life": 2.0, "initial": 1.0},
    {"name": "B", "half_life": 3.0, "initial": 0.0},
    {"name": "S", "half_life": null, "initial": 0.0}
  ],
  "edges": [
    {"parent": "A", "child": "B", "probability": 0.4},
    {"parent": "A", "child": "S", "probability": 0.6},
    {"parent": "B", "child": "S", "probability": 1.0}
  ],
  "feeds": [
    {"nuclide": "A", "rate": 0.1, "start": 1.0, "end": 4.0}
  ],
  "schedule": {"start": 0.0, "stop": 10.0, "step": 1.0},
  "windows": [{"start": 0.0, "end": 10.0}]
}
```
 
- `half_life: null` marks a stable nuclide; stable nuclides must not have
  outgoing edges. Unstable nuclides need at least one branch, and the
  branch probabilities of each parent must sum to 1.
- `initial` quantities must be non-negative; half-lives must be positive.
  At most 12 nuclides. The network must be acyclic; each decay produces
  exactly one child.
- `feeds` are optional. Each feed adds its nuclide at a constant
  non-negative `rate` over the half-open interval `[start, end)`; feeds of
  one nuclide must not overlap. All feed switch points are integrated
  exactly, so the density or ordering of output times never changes the
  results.
- `schedule` is either an explicit `"times": [...]` list (ascending) or
  `start`/`stop`/`step`. Initial quantities apply at the earliest of the
  output times, feed starts and window starts.
- `windows` define `[start, end]` accumulation intervals for decay counts
  and branch yields.
 
## Units
 
One consistent time unit is used everywhere: half-lives, schedule times,
feed interval bounds and window bounds share it. Quantities are atom
counts (or any consistent amount unit). Feed rates are amount per time
unit; activities are decays per time unit (`activity = lambda * N`).
 
## Numerical method
 
The linear system `dN/dt = A N + f(t)` is integrated analytically on each
interval where the feed vector is constant. An augmented system
`[N; 1; J]` with `dJ/dt = diag(lambda) N` is propagated by one matrix
exponential per interval, so quantities and cumulative decay counters
advance together and window integrals are never approximated by endpoint
differences.
 
The matrix exponential uses Taylor series with scaling and squaring
(tolerance 1e-14, up to 200 terms). Because no Bateman-style partial
fractions are formed, equal or arbitrarily close decay constants cause no
division by zero. The JSON report records the tolerance, the number of
Taylor terms and the number of squarings used for every interval.
 
Quantities that come out negative beyond a relative tolerance of 1e-9
abort the run with an error instead of being truncated to zero. The
conservation residual `total N(t) - initial total - cumulative feed` is
reported per output time (decays conserve nuclei because each decay
produces exactly one child).
 
## Scope and limitations
 
- Directed acyclic networks of at most 12 nuclides, one child per decay.
- Constant-rate, non-overlapping external feeds.
- Double-precision arithmetic; extremely stiff systems (half-lives
  spanning many orders of magnitude) may require many squarings and lose
  accuracy — non-convergence is reported as an error, never silently.
 
## Tests
 
```
python -m unittest discover -s tests
```
 
Covers equal-rate chains (repeated eigenvalues), branching with
converging paths, half-open feed intervals, grid-density invariance,
conservation residuals, validation failures and the CLI end to end.
