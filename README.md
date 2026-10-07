# ancestral-character-parsimony

Reconstruct ancestral discrete character states on a **fixed** rooted tree
using Sankoff dynamic programming. The tool finds a node assignment with the
minimum total change cost; it never searches or modifies the tree structure.
Results are parsimony scores (costs), not probabilities or confidences.

## Usage

```
python -m ancestral_parsimony.cli TREE.json DATA.csv
# or, after installation:
ancestral-parsimony TREE.json DATA.csv
```

Only the Python standard library is required (Python 3.12+).

## Input formats

### Tree JSON

```json
{
  "states": ["0", "1"],
  "costs": [[0, 1], [1, 0]],
  "tree": {
    "id": "root",
    "children": [
      {"id": "n1", "children": [{"id": "a"}, {"id": "b"}]},
      {"id": "c"}
    ]
  }
}
```

- `tree` is a nested object with a unique `id` per node and an optional
  `children` list. Multifurcations are allowed. Duplicate ids, cycles, and
  disconnected structures are rejected.
- `states` lists 1-6 shared state labels used by every character column.
- `costs` is the square transition cost matrix: `costs[i][j]` is the cost of
  changing from state `i` (parent) to state `j` (child) along one edge.
  Costs must be non-negative numbers, the diagonal must be zero, and
  asymmetry is allowed. Branch lengths are not used. Incomplete matrices or
  unknown states are rejected with an explicit error.

### Observations CSV

```csv
id,body_size,diet
a,0,1
b,1,?
```

- The first column holds leaf node ids; each remaining column is one
  discrete character.
- Every leaf must have exactly one row and every row must match a leaf;
  a missing row is an error, not an unknown observation.
- `?` marks an unknown observation: that leaf may take any state at no
  cost. Known leaves are fixed to their observed state.

## Method and tie-breaking

For each character column a Sankoff down-pass computes, for every node and
state, the minimum cost of the subtree below. The backtrace then produces
one **jointly optimal** assignment: the root takes the lexicographically
smallest state among those attaining the column minimum, and each child
takes the lexicographically smallest state that is optimal given its
parent's already-chosen state. States are never picked independently per
node, so the assignment is always mutually compatible. Children are
processed in sorted id order, so reordering `children` in the JSON does not
change the result.

## Output

1. **Node assignments** - reconstructed state of every node per character.
2. **Branch change costs** - per-character and total cost of every edge.
3. **Summary** - the optimal score of each column and of the whole tree.

The per-edge costs always re-add to the per-column optima, and the column
scores re-add to the reported total; the CLI verifies this internally.

## Tests

```
python -m unittest discover -s tests -v
```

Covers multifurcating trees, unknown observations, directed (asymmetric)
costs, tie-breaking rules, and input validation.
