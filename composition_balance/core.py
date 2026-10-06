"""Core math for the balance (ILR) transform. Standard library only."""

from __future__ import annotations

import math
from dataclasses import dataclass


class CompositionError(ValueError):
    """Raised when sample data fails validation."""


class TreeError(ValueError):
    """Raised when the binary partition tree is invalid."""


def close_rows(rows: list[list[float]]) -> list[list[float]]:
    """Validate values and scale each row to sum to one."""
    closed: list[list[float]] = []
    for index, row in enumerate(rows):
        if not row:
            raise CompositionError(f"row {index}: empty row")
        for value in row:
            if not math.isfinite(value):
                raise CompositionError(f"row {index}: non-finite value {value!r}")
            if value < 0.0:
                raise CompositionError(f"row {index}: negative value {value!r}")
        total = math.fsum(row)
        if total <= 0.0:
            raise CompositionError(f"row {index}: all-zero row is not allowed")
        closed.append([value / total for value in row])
    return closed


def replace_zeros(
    rows: list[list[float]], delta: float
) -> tuple[list[list[float]], list[int]]:
    """Replace zeros with ``delta`` and shrink positives proportionally.

    Returns the adjusted rows plus the zero count per row. Rows without
    zeros are returned unchanged. Requires ``count * delta < 1``.
    """
    if not math.isfinite(delta) or delta <= 0.0:
        raise CompositionError(f"replacement delta must be a positive finite number, got {delta!r}")
    adjusted: list[list[float]] = []
    counts: list[int] = []
    for index, row in enumerate(rows):
        count = sum(1 for value in row if value == 0.0)
        counts.append(count)
        if count == 0:
            adjusted.append(list(row))
            continue
        if count * delta >= 1.0:
            raise CompositionError(
                f"row {index}: {count} zeros times delta {delta!r} must be less than one"
            )
        scale = (1.0 - count * delta) / math.fsum(row)
        adjusted.append([delta if value == 0.0 else value * scale for value in row])
    return adjusted, counts


@dataclass(frozen=True)
class Node:
    """Binary partition tree node. Leaves carry a component name."""

    name: str | None = None
    left: "Node | None" = None
    right: "Node | None" = None

    @property
    def is_leaf(self) -> bool:
        return self.name is not None

    def leaves(self) -> list[str]:
        if self.is_leaf:
            return [self.name]
        return self.left.leaves() + self.right.leaves()

    def internal_preorder(self) -> list["Node"]:
        if self.is_leaf:
            return []
        return [self] + self.left.internal_preorder() + self.right.internal_preorder()


def parse_tree(data: object, component_names: list[str]) -> Node:
    """Build and validate a tree from decoded JSON.

    A leaf is a component name string or ``{"name": ...}``; an internal
    node is ``{"left": ..., "right": ...}`` with exactly two non-empty
    subtrees. Leaves must match ``component_names`` one-to-one.
    """
    if not component_names:
        raise TreeError("component name list is empty")
    if len(set(component_names)) != len(component_names):
        raise TreeError("component names must be unique")
    root = _build_node(data, "root")
    leaves = root.leaves()
    if len(leaves) < 2:
        raise TreeError("tree must contain at least two leaves")
    if sorted(leaves) != sorted(component_names):
        raise TreeError(
            f"tree leaves {sorted(leaves)} do not match components {sorted(component_names)}"
        )
    return root


def _build_node(data: object, path: str) -> Node:
    if isinstance(data, str):
        if not data:
            raise TreeError(f"{path}: empty leaf name")
        return Node(name=data)
    if isinstance(data, dict):
        if "name" in data:
            extra = set(data) - {"name"}
            if extra:
                raise TreeError(f"{path}: leaf has unexpected keys {sorted(extra)}")
            return _build_node(data["name"], path)
        keys = set(data)
        if keys != {"left", "right"}:
            raise TreeError(
                f"{path}: internal node must have exactly 'left' and 'right', got {sorted(keys)}"
            )
        left = _build_node(data["left"], f"{path}.left")
        right = _build_node(data["right"], f"{path}.right")
        if left is None or right is None:
            raise TreeError(f"{path}: internal node children must be non-empty")
        return Node(left=left, right=right)
    raise TreeError(f"{path}: unsupported node {data!r}")


def balance_coordinates(
    rows: list[list[float]], root: Node, component_names: list[str]
) -> list[list[float]]:
    """Forward transform: one balance per internal node, preorder.

    balance = sqrt(l*r/(l+r)) * ln(g_left / g_right), where l and r are
    leaf counts and g the geometric mean of each side's components.
    """
    index = {name: i for i, name in enumerate(component_names)}
    nodes = root.internal_preorder()
    sides = [
        ([index[n] for n in node.left.leaves()], [index[n] for n in node.right.leaves()])
        for node in nodes
    ]
    result: list[list[float]] = []
    for row_index, row in enumerate(rows):
        coords: list[float] = []
        for left_idx, right_idx in sides:
            for i in left_idx + right_idx:
                if row[i] <= 0.0:
                    raise CompositionError(
                        f"row {row_index}: component {component_names[i]!r} is not positive; "
                        "apply zero replacement first"
                    )
            l_count = len(left_idx)
            r_count = len(right_idx)
            log_left = math.fsum(math.log(row[i]) for i in left_idx) / l_count
            log_right = math.fsum(math.log(row[i]) for i in right_idx) / r_count
            coef = math.sqrt(l_count * r_count / (l_count + r_count))
            coords.append(coef * (log_left - log_right))
        result.append(coords)
    return result


def orthonormal_basis(root: Node, component_names: list[str]) -> list[list[float]]:
    """Orthonormal contrast matrix rows (one per internal node, preorder).

    Left leaves get +sqrt(r/(l*(l+r))), right leaves -sqrt(l/(r*(l+r))).
    """
    index = {name: i for i, name in enumerate(component_names)}
    basis: list[list[float]] = []
    for node in root.internal_preorder():
        row = [0.0] * len(component_names)
        l_count = len(node.left.leaves())
        r_count = len(node.right.leaves())
        left_value = math.sqrt(r_count / (l_count * (l_count + r_count)))
        right_value = -math.sqrt(l_count / (r_count * (l_count + r_count)))
        for name in node.left.leaves():
            row[index[name]] = left_value
        for name in node.right.leaves():
            row[index[name]] = right_value
        basis.append(row)
    return basis


def inverse_balances(
    coords: list[list[float]], root: Node, component_names: list[str]
) -> list[list[float]]:
    """Inverse transform back to positive compositions summing to one.

    Uses a softmax over V^T b with the max subtracted to avoid overflow.
    The original totals are not restored and replaced zeros stay replaced.
    """
    basis = orthonormal_basis(root, component_names)
    width = len(basis)
    size = len(component_names)
    result: list[list[float]] = []
    for row_index, coord in enumerate(coords):
        if len(coord) != width:
            raise CompositionError(
                f"row {row_index}: expected {width} coordinates, got {len(coord)}"
            )
        for value in coord:
            if not math.isfinite(value):
                raise CompositionError(f"row {row_index}: non-finite coordinate {value!r}")
        logit = [math.fsum(coord[k] * basis[k][j] for k in range(width)) for j in range(size)]
        peak = max(logit)
        weights = [math.exp(value - peak) for value in logit]
        total = math.fsum(weights)
        result.append([value / total for value in weights])
    return result


def clr_coordinates(rows: list[list[float]]) -> list[list[float]]:
    """Centered log-ratio coordinates for positive compositions."""
    result: list[list[float]] = []
    for row_index, row in enumerate(rows):
        for value in row:
            if value <= 0.0:
                raise CompositionError(f"row {row_index}: clr requires positive values")
        mean_log = math.fsum(math.log(value) for value in row) / len(row)
        result.append([math.log(value) - mean_log for value in row])
    return result


def pairwise_distances(points: list[list[float]]) -> list[list[float]]:
    """Full pairwise Euclidean distance matrix."""
    size = len(points)
    matrix = [[0.0] * size for _ in range(size)]
    for i in range(size):
        for j in range(i + 1, size):
            dist = math.sqrt(
                math.fsum((a - b) ** 2 for a, b in zip(points[i], points[j]))
            )
            matrix[i][j] = dist
            matrix[j][i] = dist
    return matrix
