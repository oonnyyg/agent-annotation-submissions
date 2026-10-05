# Hierarchical Release Anonymizer

Chooses a **global generalization scheme** for offline statistical release:
every quasi-identifier (QI) column is generalized upward by a uniform number
of steps along a validated hierarchy tree, records are grouped by the
generalized QI combination, and groups that fail the anonymity constraints
are suppressed **as a whole**. The tool searches all allowed step vectors and
picks the cost-optimal one. It is not simple masking: anonymity is defined
and checked per group, not per field.

## Input

- **CSV**: header row, first column = record id, last column = sensitive
  attribute, 1–4 middle columns = quasi-identifiers. Missing values and
  duplicate ids are rejected.
- **Hierarchy JSON**: one single-root tree per QI column, max depth 4.
  Every raw CSV value must be a **leaf**; unknown leaf values and non-tree
  hierarchies (shared/duplicate nodes) are rejected.

```json
{"columns": {"age": {"value": "any", "children": [
  {"value": "young", "children": [{"value": "20"}, {"value": "25"}]},
  {"value": "old",   "children": [{"value": "60"}, {"value": "65"}]}]}}}
```

## Constraints and cost

- Each equivalence group must have size ≥ `k` and ≥ `l` distinct sensitive
  values (`k`, `l` positive integers). Failing groups are suppressed whole —
  never only the inconvenient sensitive values.
- Total suppressed rows ≤ `budget` (non-negative integer) and at least one
  row must remain; otherwise the run fails explicitly.
- Cost = Σ(steps_j × weight_j) + suppressed × suppress_cost, with positive
  integer weights and suppression cost. Ties: fewer suppressed rows, then
  lexicographically smallest step vector in configured column order.
- Grouping, kept-row ordering and enumeration are fully sorted, so
  reordering the input rows never changes the chosen scheme.

## Usage

```
go run . anonymize -data in.csv -hierarchy h.json -k 2 -l 2 -budget 3 \
    -weights 1,1 -suppress-cost 10 -public out.csv -report report.json

go run . check -data in.csv -hierarchy h.json -k 2 -l 2 -budget 3 \
    -weights 1,1 -suppress-cost 10 -steps 1,0
```

- `anonymize` searches all step vectors and writes both outputs.
- `check` independently evaluates one given step vector: feasibility,
  failure reason, cost, and per-group details.

## Outputs

- **Public CSV** (`-public`): only generalized QI columns and the sensitive
  column. No record ids, no suppressed rows. Safe to publish *within the
  model below*.
- **Internal report JSON** (`-report`): audit-only document with the chosen
  steps, cost, per-group size/diversity checks, and per-row keep/drop
  decisions. Must not be published.

## Scope of the guarantee

The guarantees are exactly those of this task's model: group size ≥ k and
distinct sensitive values ≥ l on the chosen quasi-identifiers, under the
given hierarchies and suppression budget. This is **not** a promise against
re-identification: background knowledge, homogeneity within a group,
external data linkage, skewed sensitive values, and composition with other
releases are all out of scope. Treat the public file accordingly.

## Tests

```
go test ./...
```

Covers insufficient sensitive diversity, crossing (non-tree) hierarchies,
budget-infeasible configurations, whole-group suppression, input-order
invariance, tie-breaking, and input validation rejections.
