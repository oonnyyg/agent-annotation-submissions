"""Core math for the compositional balance transform.

Pipeline per sample row:
  1. validate (finite, non-negative, non-zero sum) and close to sum 1
  2. replace zeros with a configured positive delta, shrinking positives
  3. compute balance coordinates from a binary tree (pre-order)
  4. inverse transform reconstructs the zero-replaced composition
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field


class BalanceError(ValueError):
    """Raised for invalid compositions, trees, or coordinates."""


def validate_and_close(row, names):
    """Validate a raw row and divide by its sum. Returns a list of floats."""
    if len(row) != len(names):
        raise BalanceError(
            f"row has {len(row)} values, expected {len(names)} components"
        )
    values = []
    for name, raw in zip(names, row):
        try:
            value = float(raw)
        except (TypeError, ValueError):
            raise BalanceError(f"component {name!r}: not a number: {raw!r}")
        if not math.isfinite(value):
            raise BalanceError(f"component {name!r}: not finite: {value!r}")
        if value < 0.0:
            raise BalanceError(f"component {name!r}: negative: {value!r}")
        values.append(value)
    total = math.fsum(values)
    if total <= 0.0:
        raise BalanceError("row sums to zero; all-zero rows are rejected")
    return [v / total for v in values]


def replace_zeros(composition, delta):
    """Replace zeros with ``delta`` and shrink positives proportionally.

    The composition stays closed (sums to 1). Rows without zeros are
    returned unchanged. ``zeros * delta`` must be strictly less than 1.
    """
    if not (0.0 < delta < 1.0):
        raise BalanceError(f"delta must be in (0, 1), got {delta!r}")
    zeros = sum(1 for v in composition if v == 0.0)
    if zeros == 0:
        return list(composition)
    if zeros * delta >= 1.0:
        raise BalanceError(
            f"zero replacement overflows: {zeros} zeros * delta {delta} >= 1"
        )
    scale = (1.0 - zeros * delta) / math.fsum(composition)
    return [delta if v == 0.0 else v * scale for v in composition]


@dataclass
class TreeNode:
    """Binary tree node; leaves carry a component ``name``."""

    name: str | None = None
    left: "TreeNode | None" = None
    right: "TreeNode | None" = None

    @property
    def is_leaf(self):
        return self.left is None and self.right is None

    def leaves(self):
        if self.is_leaf:
            return [self.name]
        return self.left.leaves() + self.right.leaves()

    def internal_nodes_preorder(self):
        if self.is_leaf:
            return []
        return (
            [self]
            + self.left.internal_nodes_preorder()
            + self.right.internal_nodes_preorder()
        )


def parse_tree(data, names):
    """Build a TreeNode from JSON data and check leaves match ``names``.

    Accepted JSON shapes: a string (leaf) or an object with exactly the
    keys ``left`` and ``right`` (internal node, both non-empty).
    """
    root = _parse_node(data)
    leaves = root.leaves()
    if any(leaf is None for leaf in leaves):
        raise BalanceError("tree leaf is missing a component name")
    if sorted(leaves) != sorted(names):
        raise BalanceError(
            f"tree leaves {sorted(leaves)} do not match components {sorted(names)}"
        )
    if len(set(leaves)) != len(leaves):
        raise BalanceError("tree leaves contain duplicate component names")
    return root


def _parse_node(data):
    if isinstance(data, str):
        if not data:
            raise BalanceError("empty leaf name in tree")
        return TreeNode(name=data)
    if isinstance(data, dict):
        if set(data) != {"left", "right"}:
            raise BalanceError(
                "internal node must have exactly 'left' and 'right' keys, "
                f"got {sorted(data)}"
            )
        left = _parse_node(data["left"])
        right = _parse_node(data["right"])
        return TreeNode(left=left, right=right)
    raise BalanceError(f"invalid tree node: {data!r}")


def _node_coefficient(node):
    r = len(node.left.leaves())
    s = len(node.right.leaves())
    return math.sqrt(r * s / (r + s))


def forward(composition, root, names):
    """Balance coordinates of a closed, positive composition, pre-order.

    Each coordinate is sqrt(r*s/(r+s)) * ln(g_left / g_right) where g is
    the geometric mean of the components on that side of the node.
    """
    index = {name: i for i, name in enumerate(names)}
    logs = [math.log(v) for v in composition]
    coords = []
    for node in root.internal_nodes_preorder():
        left_logs = [logs[index[n]] for n in node.left.leaves()]
        right_logs = [logs[index[n]] for n in node.right.leaves()]
        mean_left = math.fsum(left_logs) / len(left_logs)
        mean_right = math.fsum(right_logs) / len(right_logs)
        coords.append(_node_coefficient(node) * (mean_left - mean_right))
    return coords


def build_basis(root, names):
    """Orthonormal basis rows (one per internal node, pre-order).

    Row k has +coef/r on left leaves and -coef/s on right leaves, so that
    forward(x) == basis @ clr(x) and inverse applies the transpose.
    """
    index = {name: i for i, name in enumerate(names)}
    rows = []
    for node in root.internal_nodes_preorder():
        coef = _node_coefficient(node)
        row = [0.0] * len(names)
        for n in node.left.leaves():
            row[index[n]] = coef / len(node.left.leaves())
        for n in node.right.leaves():
            row[index[n]] = -coef / len(node.right.leaves())
        rows.append(row)
    return rows


def clr(composition):
    """Centred log-ratio coordinates of a closed, positive composition."""
    logs = [math.log(v) for v in composition]
    mean = math.fsum(logs) / len(logs)
    return [v - mean for v in logs]


def inverse(coords, root, names):
    """Reconstruct the (zero-replaced) composition from coordinates.

    Returns a positive composition summing to 1. Uses the orthonormal
    basis from the same tree; the exponential is computed on values
    shifted by their max to avoid overflow.
    """
    basis = build_basis(root, names)
    if len(coords) != len(basis):
        raise BalanceError(
            f"got {len(coords)} coordinates, expected {len(basis)}"
        )
    clr_values = [0.0] * len(names)
    for coord, row in zip(coords, basis):
        if not math.isfinite(coord):
            raise BalanceError(f"coordinate not finite: {coord!r}")
        for j in range(len(names)):
            clr_values[j] += coord * row[j]
    shift = max(clr_values)
    exp_values = [math.exp(v - shift) for v in clr_values]
    total = math.fsum(exp_values)
    return [v / total for v in exp_values]


def pairwise_euclidean(rows):
    """Upper-triangular pairwise Euclidean distances as (i, j, dist)."""
    out = []
    for i in range(len(rows)):
        for j in range(i + 1, len(rows)):
            dist = math.sqrt(
                math.fsum((a - b) ** 2 for a, b in zip(rows[i], rows[j]))
            )
            out.append((i, j, dist))
    return out
