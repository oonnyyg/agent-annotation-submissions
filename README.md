# discrete-bayes-query

A standard-library-only command line tool that answers joint conditional
probability queries on discrete Bayesian networks using variable elimination.

## Usage

```
python -m bayesnet NETWORK.json QUERIES.jsonl
```

One JSON result is printed per query line. Queries that fail (invalid spec or
zero-probability evidence) produce an `{"error": ...}` line and set the exit
code to 1; other queries still run. Evidence never leaks between queries.

## Network format (JSON)

```json
{
  "variables": {"A": ["a0", "a1"], "B": ["b0", "b1"], "C": ["c0", "c1"]},
  "cpt": {
    "A": {"parents": [], "rows": [
      {"parent_states": {}, "probs": {"a0": 0.6, "a1": 0.4}}
    ]},
    "C": {"parents": ["A", "B"], "rows": [
      {"parent_states": {"A": "a0", "B": "b0"}, "probs": {"c0": 0.9, "c1": 0.1}}
    ]}
  }
}
```

Constraints (validated, never auto-corrected):

- At most 12 variables, each with at most 4 unique string states.
- The graph must be acyclic; parents must be declared variables.
- Every CPT must contain exactly one row per parent-state combination
  (missing or duplicate combinations are errors).
- Each row lists exactly the variable's states; probabilities must be
  non-negative numbers summing to 1 within `1e-9`.

## Query format (JSONL, one object per line)

```json
{"target": "A", "evidence": {"C": "c1"}}
{"target": ["A", "B"], "evidence": {"C": "c1"}, "order": ["B"]}
{"target": "C"}
```

- `target`: one variable name or a list of names (joint query).
- `evidence`: optional object of observed variable states; must not overlap
  the target. Empty evidence is supported.
- `order`: optional elimination order covering every hidden variable exactly
  once. When omitted, the next variable is chosen greedily to minimize the
  number of entries in the resulting factor product; ties break by variable
  name.

## Output

Each result contains:

- `evidence_probability`: P(evidence); if it is zero the query fails with an
  explicit error instead of returning a uniform distribution.
- `distribution`: normalized joint distribution over the target variables.
- `map_assignment`: the most likely joint assignment of the target variables,
  computed by summing out hidden variables first and then taking the argmax
  over the joint target table (never per-variable maxima). Ties break by
  lexicographic order of variable names and state names.
- `summary`: the actual `elimination_order`, the `max_factor_size` seen
  during elimination, and the list of hidden variables.

## Tolerances

- CPT row sums must equal 1 within `1e-9`.
- MAP ties are decided with a `1e-12` margin.
- Different valid elimination orders agree to within normal floating-point
  error (tests assert agreement to 12 decimal places).

## Tests

```
python -m unittest discover -s tests
```
