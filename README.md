# Pedigree Genotype Inference (Teaching Tool)

Exact genotype-probability inference for a **single biallelic locus**
(genotypes `AA` / `AB` / `BB`) on a small pedigree, using only the Python
standard library. Built for animal-breeding **teaching data**; it does not
infer disease risk or real breeding value.

## Model and assumptions

- **Founder independence**: individuals without parents are mutually
  independent and follow a Hardy-Weinberg prior derived from the A-allele
  frequency `allele_freq` (strictly between 0 and 1):
  `P(AA)=p^2`, `P(AB)=2pq`, `P(BB)=q^2`.
- **Mendelian segregation**: each non-founder receives one allele uniformly
  at random from each parent. Parents must be two distinct, registered
  individuals; forward references (use before definition) are allowed.
  Duplicate ids and kinship cycles are rejected.
- **Noisy observations**: an observed genotype matches the true genotype
  with probability `1 - error_rate`; the other two genotypes share
  `error_rate` equally (`error_rate` in `[0, 1)`). Missing observations
  contribute no evidence, and an observation never fixes the true genotype.
- **Exact computation**: the joint distribution over all `3^n` genotype
  assignments (n <= 10 individuals, i.e. at most 59 049 states) is summed
  exactly and normalized. If the evidence probability is zero, the
  observations are mutually contradictory; this is reported as
  `contradictory_observations: true` and no posterior is fabricated.
- **Offspring prediction**: `--predict FATHER MOTHER` averages Mendelian
  transmission probabilities over the parents' **joint** posterior (their
  marginal posteriors are correlated when parents are related, so they are
  never simply multiplied). Prediction does not modify the pedigree and the
  hypothetical child is never conditioned on as an observation.

## Computational scale

Enumeration costs `O(3^n * n)` time and `O(3^n)` memory. This is exact and
instant for the supported maximum of 10 individuals; larger pedigrees would
need approximate or peeling-based algorithms, which are out of scope.

## Input format (JSON)

```json
{
  "allele_freq": 0.5,
  "error_rate": 0.05,
  "individuals": [
    {"id": "calf", "parents": ["sire", "dam"], "observation": "BB"},
    {"id": "sire", "observation": "AB"},
    {"id": "dam"}
  ]
}
```

`parents` and `observation` are optional; omit `parents` for founders and
`observation` for unobserved individuals.

## Usage

```sh
python -m pedigree_inference examples/trio.json --predict sire dam --csv summary.csv
```

- JSON posterior distributions and the evidence probability go to stdout.
- `--csv PATH` writes a per-individual summary table (plus the predicted
  offspring row when `--predict` is given).

## Tests

```sh
python -m unittest discover -s tests
```

Covers missing observations, kinship-driven evidence propagation, noisy
observations, zero-error contradictions, and joint-posterior offspring
prediction.
