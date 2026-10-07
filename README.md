# BET Surface Area Fit

Stdlib-only Python tool that turns adsorption-branch data into a reviewable
BET fit interval and surface-area report. Instead of hiding unsuitable data
behind a fixed pressure range, it enumerates candidate intervals, screens
them, and reports every candidate with its rejection reasons.

## Input

CSV with one row per measurement point (multiple samples per file):

```csv
sample,relative_pressure,quantity_mol_per_g
zeolite-a,0.05000,2.51200e-03
```

- `relative_pressure`: p/p0, strictly between 0 and 1
- `quantity_mol_per_g`: adsorbed amount q in mol/g (positive, finite)

Each sample is sorted by pressure. Duplicate pressures and non-finite
values abort with the offending CSV line numbers.

JSON config:

```json
{
  "cross_section_nm2": 0.162,
  "min_points": 4,
  "interval": [0.05, 0.30]
}
```

- `cross_section_nm2` (required): adsorbate molecular cross-section in nm^2
  (0.162 for N2 at 77 K).
- `min_points` (default 4): minimum points per fitted interval.
- `interval` (optional): manual closed interval `[lo, hi]` in p/p0. Manual
  intervals are screened exactly like enumerated ones; they cannot skip
  the checks. Omit it to enumerate all contiguous intervals.

## Method

BET linearization (x = p/p0):

```
y = 1 / (q * (1/p - 1)) = p / (q * (1 - p))
y = slope * x + intercept
```

Least-squares fit per candidate interval, then:

```
n_m = 1 / (slope + intercept)        monolayer capacity, mol/g
C   = 1 + slope / intercept          BET constant
S   = n_m * N_A * sigma * 1e-18      specific surface area, m^2/g
```

with `N_A = 6.02214076e23 mol^-1` (exact, SI) and `sigma` the cross-section
converted from nm^2 to m^2 (1 nm^2 = 1e-18 m^2). Quantities in mol/g times
molecules per mol times m^2 per molecule give m^2/g directly.

## Screening and selection

An interval is accepted only if all of the following hold:

1. intercept > 0
2. monolayer capacity n_m > 0
3. BET constant C > 0
4. q * (1 - p) does not decrease over the interval (Rouquerol criterion)

Automatic selection picks the accepted interval with the highest R^2;
ties break toward more points, then toward the lower starting pressure.
Intervals where R^2 is undefined (zero ordinate variance) never
participate in automatic selection. If nothing qualifies, the sample
fails explicitly and the CLI exits non-zero.

These rules are this tool's screening conditions only; they do not claim
to certify full BET applicability for a material.

## Usage

```
python -m bet_fit data.csv config.json [-o report.txt]
```

Exit codes: 0 = all samples fitted, 1 = at least one sample failed,
2 = input/config error. The report shows, per sample, the selected range
with parameters and per-point residuals, plus a table of every candidate
interval with acceptance status or rejection reasons.

## Example

```
python examples/make_example.py
python -m bet_fit examples/isotherms.csv examples/config.json -o examples/report.txt
```

## Tests

```
python -m unittest discover -s tests
```

Covers recovery of a known synthetic BET isotherm, tie-breaking, manual
interval screening, Rouquerol violations, degenerate data (undefined R^2,
non-positive intercept), and CSV/config validation.
