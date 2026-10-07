# discrete-mass-transport

Pure-stdlib Python tool that moves integer mass between two discrete
distributions on a 2D integer grid at minimum total cost, and prints the
resulting transport plan. No optimization packages required.

## Input formats

**Distribution CSV** (one file per side, header required):

```csv
id,x,y,mass
depot-a,0,0,7
```

- `id`: unique identifier within the file. Different ids may share the
  same coordinates.
- `x`, `y`: integer grid coordinates.
- `mass`: non-negative integer. Zero-mass locations are accepted and kept
  in the input, they simply carry no flow.

Constraints, validated before solving: at most 32 locations per side,
equal and strictly positive total mass on both sides, no duplicate ids,
no negative masses. Invalid data is rejected with an error message and
exit code 2.

**Config JSON**:

```json
{
  "metric": "manhattan",
  "disabled": [["depot-a", "store-x"]]
}
```

- `metric` (optional, default `"manhattan"`): unit transport cost between
  a source at `(x1, y1)` and a target at `(x2, y2)`.
  - `manhattan`: `|x1 - x2| + |y1 - y2|`
  - `sqeuclidean`: `(x1 - x2)^2 + (y1 - y2)^2`
- `disabled` (optional): list of `[source_id, target_id]` pairs that must
  not carry any flow. Unknown ids are rejected.

## Cost model

Moving one unit of mass along a path costs the path's unit cost; the total
cost is the sum of `amount * unit_cost` over all used paths. Mass may be
split: one source can serve several targets and vice versa, as long as
each source ships exactly its mass and each target receives exactly its
mass. The reported average cost is `total_cost / total_mass`.

## Solver

The problem is solved exactly as a minimum-cost flow (successive shortest
paths with Johnson potentials), not by greedy nearest-target assignment —
earlier allocations are revised whenever that lowers the global cost.
Masses remain integer arc capacities, so large quantities move in bulk
and are never expanded into per-unit objects. If disabled paths make it
impossible to move all mass, the tool reports `INFEASIBLE` (exit code 1)
instead of presenting a partial flow as a solution.

Tie-breaking is deterministic: nodes and arcs are scanned in sorted-id
order, so identical inputs always produce the identical plan, which is
the lexicographically smallest optimal plan in `(source id, target id)`
order.

## Running

```sh
python -m mass_transport examples/sources.csv examples/targets.csv examples/config.json
```

Output: per-path integer amounts with unit and total cost, total and
average cost, and a conservation check confirming every source and target
matches its input mass.

## Tests

```sh
python -m unittest discover -s tests -v
```

Covers input validation, a greedy-assignment trap, disabled paths,
infeasibility, mass scaling (bulk, not per-unit), deterministic
tie-breaking, and per-point conservation.
