# sam_coverage

Sequencing alignment QC tool (Python standard library only). Computes
target-region coverage from a SAM file, counting **only truly aligned
reference bases** (`M`, `=`, `X`) so deletions (`D`) and intron-skipping
gaps (`N`) are never mistaken for effective coverage. No `samtools`, no
BAM support — plain SAM text only.

## Usage

```
python -m sam_coverage --sam in.sam --bed targets.bed --config config.json \
    --csv per_target.csv --json summary.json
```

- `--sam`: SAM file; reference lengths come from `@SQ` `LN` tags.
- `--bed`: target regions, 0-based half-open (`start` inclusive, `end`
  exclusive), optional 4th column is the target id.
- `--config`: JSON object, e.g. `{"min_mapq": 20}`.
- `--csv`: per-target CSV with `mean_depth`, `fraction_covered_1x` and a
  JSON depth histogram (depth -> number of target bases, zero depth
  included).
- `--json`: splice-junction support (from `N` operations only, never
  confused with `D`) plus per-stage filtering counts.

## Coordinate conventions

- BED targets are 0-based half-open. SAM `POS` is 1-based and is
  converted to the same 0-based system before any overlap math.
- Targets with unknown references, out-of-bounds coordinates, or
  duplicated target ids are rejected. Targets may overlap; each is
  counted independently.
- Reverse-strand alignments are **not** re-reversed; reference
  coordinates are used exactly as reported.

## Filtering order

Records are evaluated in this exact order; a record dropped at one stage
is counted there and never reaches later stages:

1. `FLAG & 0x4` unmapped
2. non-primary: secondary (`0x100`) or supplementary (`0x800`)
3. QC-fail (`0x200`) or duplicate (`0x400`)
4. `MAPQ < min_mapq` from the JSON config
5. record validity: unknown reference, out-of-bounds alignment, invalid
   CIGAR, or CIGAR query consumption not matching the non-`*` SEQ length

Each retained record contributes independently — paired-end overlaps are
**not** merged.

## CIGAR semantics

- `M`, `=`, `X`: +1 coverage per aligned reference base.
- `D`, `N`: advance the reference cursor only (no coverage).
- `I`, `S`: consume query only. `H`, `P`: consume nothing.
- `N` intervals additionally aggregate junction support keyed by
  (reference, start, end); transcript strand is not inferred.

## Coverage algorithm

Per target, coverage is computed with interval events (sweep line), not
per-base arrays sized by chromosome length. Zero-depth bases are counted
in the histogram explicitly.

## Tests

```
python -m unittest discover -s tests
```
