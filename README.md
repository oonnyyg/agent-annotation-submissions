# discrete-bayes-query

Query a discrete Bayesian network with evidence, using variable elimination.
Standard library only; Python >= 3.12.

## Usage

```
python -m bayes NETWORK.json QUERIES.jsonl
python -m bayes NETWORK.json            # read queries from stdin
python -m unittest discover -s tests    # run tests
```

## Network JSON format

```json
{
  "variables": [
    {
      "name": "Alarm",
      "states": ["off", "on"],
      "parents": ["Burglary", "Earthquake"],
      "cpt": [
        {"when": ["no", "no"], "probs": {"off": 0.999, "on": 0.001}}
      ]
    }
  ]
}
```

- At most 12 variables, each with at most 4 states; the graph must be a DAG.
- `when` lists one parent state per parent, in `parents` order; root nodes use `[]`.
- The CPT must contain exactly one row per parent-state combination, and each
  row must cover exactly the variable's states.
- Validation is strict: duplicate states, missing parent-state combinations,
  negative probabilities, and rows whose probabilities do not sum to 1
  (tolerance `1e-9`) are rejected; invalid tables are never auto-corrected.

## Query JSONL format

One JSON object per line:

```json
{"targets": ["Burglary"], "evidence": {"JohnCalls": "yes"}, "elimination_order": ["Alarm", "Earthquake"]}
```

- `targets` (or singular `target`) names the query variables; `evidence` maps
  observed variables to states. The two sets must not overlap. Empty evidence
  (`{}` or omitted) is allowed.
- `elimination_order` is optional. If given, it must cover exactly the hidden
  variables (neither targets nor evidence). If omitted, the tool greedily
  eliminates next the variable whose product factor has the fewest entries
  (product of state counts of its scope), breaking ties by variable name.

## Output

One JSON object per query line: `evidence_probability`, normalized
`distribution` over the target tuple, `map_assignment` (the most probable
joint assignment of the targets, computed by summing out hidden variables
first and then taking the argmax — never per-variable maxima; ties resolve to
the lexicographically smallest state tuple with targets sorted by name), and a
`summary` with the actual `elimination_order` and the `max_factor_size`
(largest intermediate factor, in entries) seen during elimination.

Evidence with zero probability is reported as an error (`"evidence has zero
probability"`); a uniform distribution is never substituted. Each query line
is evaluated independently — no evidence leaks between queries.

## Numeric tolerance

Inference uses plain floats. CPT row sums are validated with absolute
tolerance `1e-9`. Different legal elimination orders yield results that agree
within `1e-9` relative / `1e-12` absolute tolerance (verified by tests).

See `examples/network.json` and `examples/queries.jsonl` for a runnable
example (the classic burglary/earthquake alarm network).
