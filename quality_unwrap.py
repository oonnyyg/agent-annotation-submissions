"""Quality-guided 2D phase unwrapping using only the Python standard library.

Reads phase, quality and mask CSV grids, builds a maximum spanning forest
over 4-connected valid pixels ordered by edge quality, propagates unwrapped
values along tree edges, and reports any adjacency constraints that the
result cannot satisfy instead of smoothing them away.
"""

from __future__ import annotations

import csv
import json
import math
import sys

TWO_PI = 2.0 * math.pi


def normalize_phase(value: float) -> float:
    """Map a phase value into the half-open interval [-pi, pi)."""
    wrapped = math.fmod(value + math.pi, TWO_PI)
    if wrapped < 0.0:
        wrapped += TWO_PI
    result = wrapped - math.pi
    if result >= math.pi:
        result -= TWO_PI
    return result


def diagnostics_dict(result: UnwrapResult) -> dict:
    components = []
    for label, root in enumerate(result.roots):
        components.append(
            {
                "label": label,
                "root": [root // result.cols, root % result.cols],
                "root_quality": result.quality[root],
            }
        )
    return {
        "rows": result.rows,
        "cols": result.cols,
        "component_count": len(result.roots),
        "components": components,
        "tree_edge_count": len(result.tree_edges),
        "conflict_count": len(result.conflicts),
        "conflicts": result.conflicts,
        "note": (
            "Absolute cycle counts between separate components are "
            "undetermined; conflicts list adjacency constraints the "
            "unwrapped result does not satisfy."
        ),
    }


def main(argv: list[str] | None = None) -> int:
    import argparse

    parser = argparse.ArgumentParser(
        description="Quality-guided 2D phase unwrapping (standard library only)."
    )
    parser.add_argument("phase", help="CSV grid of wrapped phase values")
    parser.add_argument("quality", help="CSV grid of non-negative quality values")
    parser.add_argument(
        "--mask",
        help="optional CSV grid; non-zero cells are excluded from unwrapping",
    )
    parser.add_argument("--out-unwrapped", required=True, help="output CSV of unwrapped phase")
    parser.add_argument("--out-labels", required=True, help="output CSV of component labels")
    parser.add_argument("--diagnostics", required=True, help="output JSON diagnostics file")
    args = parser.parse_args(argv)

    phase = read_grid(args.phase)
    quality = read_grid(args.quality)
    mask = read_grid(args.mask) if args.mask else None
    result = unwrap(phase, quality, mask)

    rows, cols = result.rows, result.cols
    unwrapped_grid: list[list[float | int]] = []
    label_grid: list[list[int]] = []
    for r in range(rows):
        u_row: list[float | int] = []
        l_row: list[int] = []
        for c in range(cols):
            idx = r * cols + c
            if result.valid[idx]:
                u_row.append(result.unwrapped[idx])
                l_row.append(result.labels[idx])
            else:
                u_row.append("")
                l_row.append(-1)
        unwrapped_grid.append(u_row)
        label_grid.append(l_row)

    write_grid(args.out_unwrapped, unwrapped_grid)
    write_grid(args.out_labels, label_grid)
    with open(args.diagnostics, "w", encoding="utf-8") as handle:
        json.dump(diagnostics_dict(result), handle, indent=2)
        handle.write("\n")
    return 0


def read_grid(path: str) -> list[list[float]]:
    """Read a CSV file of numbers into a list of rows."""
    rows: list[list[float]] = []
    with open(path, newline="", encoding="utf-8") as handle:
        for line_number, record in enumerate(csv.reader(handle), start=1):
            if not record:
                continue
            row: list[float] = []
            for field in record:
                text = field.strip()
                if text == "":
                    raise ValueError(f"{path}:{line_number}: empty field")
                row.append(float(text))
            rows.append(row)
    if not rows:
        raise ValueError(f"{path}: no data rows")
    width = len(rows[0])
    for line_number, row in enumerate(rows, start=1):
        if len(row) != width:
            raise ValueError(f"{path}:{line_number}: ragged row")
    return rows


def write_grid(path: str, rows: list[list[float | int]]) -> None:
    with open(path, "w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        for row in rows:
            writer.writerow(row)


class DisjointSet:
    def __init__(self, size: int) -> None:
        self.parent = list(range(size))
        self.rank = [0] * size

    def find(self, item: int) -> int:
        root = item
        while self.parent[root] != root:
            root = self.parent[root]
        while self.parent[item] != root:
            self.parent[item], item = root, self.parent[item]
        return root

    def union(self, a: int, b: int) -> bool:
        ra, rb = self.find(a), self.find(b)
        if ra == rb:
            return False
        if self.rank[ra] < self.rank[rb]:
            ra, rb = rb, ra
        self.parent[rb] = ra
        if self.rank[ra] == self.rank[rb]:
            self.rank[ra] += 1
        return True


class UnwrapResult:
    def __init__(self, rows: int, cols: int) -> None:
        self.rows = rows
        self.cols = cols
        self.valid: list[bool] = []
        self.phase: list[float] = []
        self.quality: list[float] = []
        self.unwrapped: list[float | None] = []
        self.labels: list[int] = []
        self.cycle_offsets: list[int | None] = []
        self.roots: list[int] = []
        self.tree_edges: list[tuple[int, int, float]] = []
        self.conflicts: list[dict] = []


def unwrap(
    phase_grid: list[list[float]],
    quality_grid: list[list[float]],
    mask_grid: list[list[float]] | None = None,
) -> UnwrapResult:
    rows = len(phase_grid)
    cols = len(phase_grid[0])
    for name, grid in (("quality", quality_grid), ("mask", mask_grid)):
        if grid is None:
            continue
        if len(grid) != rows or any(len(r) != cols for r in grid):
            raise ValueError(f"{name} grid shape does not match phase grid")

    size = rows * cols
    result = UnwrapResult(rows, cols)
    result.valid = [True] * size
    result.phase = [0.0] * size
    result.quality = [0.0] * size
    result.labels = [-1] * size
    result.unwrapped = [None] * size
    result.cycle_offsets = [None] * size

    for r in range(rows):
        for c in range(cols):
            idx = r * cols + c
            masked = mask_grid is not None and mask_grid[r][c] != 0.0
            if masked:
                result.valid[idx] = False
                continue
            p = phase_grid[r][c]
            q = quality_grid[r][c]
            if not math.isfinite(p):
                raise ValueError(f"phase at ({r}, {c}) is not finite")
            if not math.isfinite(q) or q < 0.0:
                raise ValueError(f"quality at ({r}, {c}) must be finite and non-negative")
            result.phase[idx] = normalize_phase(p)
            result.quality[idx] = q

    # Build 4-connected edges, oriented from the smaller index to the larger.
    edges: list[tuple[float, int, int]] = []
    for r in range(rows):
        for c in range(cols):
            u = r * cols + c
            if not result.valid[u]:
                continue
            for dr, dc in ((0, 1), (1, 0)):
                nr, nc = r + dr, c + dc
                if nr >= rows or nc >= cols:
                    continue
                v = nr * cols + nc
                if not result.valid[v]:
                    continue
                weight = min(result.quality[u], result.quality[v])
                edges.append((weight, u, v))

    # Deterministic order: quality descending, ties by endpoint indices.
    edges.sort(key=lambda e: (-e[0], e[1], e[2]))

    dsu = DisjointSet(size)
    adjacency: list[list[tuple[int, float]]] = [[] for _ in range(size)]
    for weight, u, v in edges:
        if dsu.union(u, v):
            delta = normalize_phase(result.phase[v] - result.phase[u])
            result.tree_edges.append((u, v, weight))
            adjacency[u].append((v, delta))
            adjacency[v].append((u, -delta))

    # Label components and pick roots: highest quality, then row-major order.
    component_members: dict[int, list[int]] = {}
    for idx in range(size):
        if result.valid[idx]:
            component_members.setdefault(dsu.find(idx), []).append(idx)

    ordered_components = sorted(
        component_members.values(),
        key=lambda members: min(members),
    )
    for label, members in enumerate(ordered_components):
        root = min(members, key=lambda i: (-result.quality[i], i))
        result.roots.append(root)
        result.unwrapped[root] = result.phase[root]
        stack = [root]
        while stack:
            u = stack.pop()
            result.labels[u] = label
            for v, delta in adjacency[u]:
                if result.unwrapped[v] is None:
                    result.unwrapped[v] = result.unwrapped[u] + delta
                    stack.append(v)

    # Integer cycle offset of every pixel relative to its normalized input.
    for idx in range(size):
        if result.valid[idx]:
            offset = (result.unwrapped[idx] - result.phase[idx]) / TWO_PI
            result.cycle_offsets[idx] = int(round(offset))

    # Verify every original adjacency edge, including non-tree edges.
    for weight, u, v in edges:
        principal = normalize_phase(result.phase[v] - result.phase[u])
        actual = result.unwrapped[v] - result.unwrapped[u]
        cycles = int(round((actual - principal) / TWO_PI))
        if cycles != 0:
            result.conflicts.append(
                {
                    "from": [u // cols, u % cols],
                    "to": [v // cols, v % cols],
                    "cycle_difference": cycles,
                    "edge_quality": weight,
                }
            )

    return result


if __name__ == "__main__":
    sys.exit(main())
