# Pedigree Genotype Inference (Teaching Tool)

Exact genotype probability inference for a **single biallelic locus**
(genotypes `AA`, `AB`, `BB`) on a small pedigree, intended for animal
breeding **teaching data**. It does **not** infer disease risk or actual
breeding value.

## Usage

```
python cli.py input.json [--json out.json] [--csv out.csv]
```

- Prints the JSON posterior to stdout.
- `--json` writes the full JSON posterior (per-individual distributions,
  evidence probability, optional offspring prediction).
- `--csv` writes a per-individual summary table (plus an offspring row when
  a prediction is requested).

Run tests with `python -m unittest test_pedigree`.

## Input format

```json
{
  "allele_frequency": 0.4,
  "error_rate": 0.05,
  "individuals": [
    {"id": "calf", "parents": ["dam", "sire"], "observation": "AB"},
    {"id": "dam", "observation": "AB"},
    {"id": "sire"}
  ],
  "predict_offspring": ["dam", "sire"]
}
```

- `allele_frequency`: frequency of allele `A`, strictly between 0 and 1.
- `error_rate`: genotyping error rate, in `[0, 1)`; defaults to 0.
- `individuals`: at most 10 entries, unique `id`s. `parents` is either
  omitted (founder) or exactly two distinct, registered ids. Forward
  references are allowed (parents may be defined later in the list).
  Relationship cycles and duplicate ids are rejected.
- `observation`: `AA`/`AB`/`BB` or omitted/`null` (missing).
- `predict_offspring` (optional): two distinct existing individual ids.

## Model

- **Founders** are mutually independent with Hardy-Weinberg priors
  `(p^2, 2p(1-p), (1-p)^2)` from the given `A` allele frequency.
- **Offspring** inherit one random allele from each parent (Mendelian
  transmission).
- **Observations** are noisy: an observed genotype is correct with
  probability `1 - error_rate`; each of the other two genotypes gets
  `error_rate / 2`. A missing observation contributes no evidence, and an
  observation is never treated as the fixed true genotype.
- **Inference** sums exactly over the full joint genotype state space and
  normalizes, yielding per-individual posterior distributions and the
  evidence probability `P(observations)`. If the evidence probability is
  zero, the observations are mutually contradictory under the model; the
  tool reports an error instead of fabricating a uniform distribution.
- **Offspring prediction** for two existing individuals uses their **joint
  posterior** genotype distribution (summing over all other individuals),
  not the product of their marginal posteriors, because related individuals
  are correlated. The hypothetical offspring is not added to the pedigree
  and contributes no observation, so the original posteriors are unchanged.

## Assumptions and scale

- Independence assumptions: founders are independent; conditional on
  parental genotypes, transmissions to different offspring are independent;
  genotyping errors are independent across individuals.
- Computational scale: exact enumeration costs `O(3^n)` joint states for
  `n` individuals (at most `3^10 = 59049` states here), which is why the
  pedigree size is capped at 10. Larger pedigrees would require approximate
  or message-passing algorithms.
