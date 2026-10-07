# Quality-Guided 2D Phase Unwrapping

Pure standard-library Python tool that recovers region-wise continuous
phase from a wrapped phase grid, and explicitly reports adjacency
constraints that cannot be satisfied instead of smoothing them away.

## Usage

```
python quality_unwrap.py PHASE.csv QUALITY.csv [--mask MASK.csv] \
    --out-unwrapped OUT.csv --out-labels LABELS.csv --diagnostics DIAG.json
```

- `PHASE.csv` / `QUALITY.csv` / `MASK.csv`: same-shaped CSV grids.
  Non-zero mask cells are excluded entirely (their phase values are never
  read, so masked cells may contain non-finite placeholders).
- Valid pixels must have finite phase and finite, non-negative quality.
- `--out-unwrapped`: unwrapped phase per pixel (empty for masked cells).
- `--out-labels`: connected-component label per pixel (`-1` for masked).
- `--diagnostics`: JSON with component roots, tree edge count, and the
  list of conflicting adjacency edges.

## Algorithm

1. **Normalization.** Every valid phase value is mapped into the
   half-open interval `[-pi, pi)`. The boundary rule is: `+pi` wraps to
   `-pi`, so the interval is closed at `-pi` and open at `+pi`. The same
   rule is applied to pairwise phase differences, giving each directed
   edge a principal difference in `[-pi, pi)`.
2. **Edges.** Only 4-connected (up/down/left/right) valid neighbors are
   linked. Edge quality is the smaller of the two endpoint qualities.
3. **Maximum spanning forest.** Kruskal's algorithm processes edges by
   descending quality; ties are broken by the row-major indices of the
   (smaller, then larger) endpoints, so the result never depends on
   container iteration order.
4. **Roots.** Each connected component is rooted at its highest-quality
   pixel; ties pick the smallest row-major position. The root keeps its
   normalized input phase as its unwrapped value.
5. **Propagation.** Every tree edge is oriented from the smaller index
   to the larger one for computing the principal difference; walking the
   edge backwards uses the negated difference. Unwrapped values
   accumulate along tree paths, preserving an integer-cycle offset
   `k_i = (unwrapped_i - normalized_i) / 2pi` for every pixel.
6. **Verification.** All original adjacency edges (tree and non-tree)
   are re-checked: the unwrapped difference is compared against the
   principal difference, and any nonzero whole-cycle gap is emitted in
   the diagnostics `conflicts` list with endpoint positions and the
   integer cycle difference. A successfully built forest does **not**
   imply all constraints are satisfied.

## Interpretation

- Isolated pixels form their own single-pixel regions.
- Masked cells break connectivity; no path crosses them.
- Absolute cycle counts between different components are undetermined
  and are never claimed by the output.
- Conflicting non-tree edges are kept in the result and flagged, never
  averaged away.

## Tests

```
python -m unittest test_quality_unwrap -v
```

Covers cross-cycle ramps, mask-split components, isolated pixels,
inconsistent residue loops, and input validation.
