"""Deterministic Newick and CSV export."""

from __future__ import annotations

import csv
import io
import re

_SAFE = re.compile(r"[A-Za-z0-9.\-]+")


def format_length(value):
    """Exact decimal string for a Fraction with a terminating expansion."""
    num, den = value.numerator, value.denominator
    twos = fives = 0
    rest = den
    while rest % 2 == 0:
        rest //= 2
        twos += 1
    while rest % 5 == 0:
        rest //= 5
        fives += 1
    if rest != 1:  # non-terminating; should not happen with decimal inputs
        return f"{float(value):.12g}"
    scale = max(twos, fives)
    scaled = abs(num) * (2 ** (scale - twos)) * (5 ** (scale - fives))
    digits = str(scaled).rjust(scale + 1, "0")
    if scale:
        digits = digits[:-scale] + "." + digits[-scale:]
        digits = digits.rstrip("0").rstrip(".")
    return ("-" if num < 0 else "") + digits


def quote_label(name):
    """Quote unless safe; underscores force quoting to preserve identity."""
    if name and _SAFE.fullmatch(name):
        return name
    return "'" + name.replace("'", "''") + "'"


def to_newick(root):
    """Serialize with subtrees ordered by smallest leaf name."""

    def emit(node):
        out = ""
        if node.children:
            ordered = sorted(node.children, key=lambda c: c.min_leaf_name())
            out += "(" + ",".join(emit(child) for child in ordered) + ")"
        if node.name:
            out += quote_label(node.name)
        if node.parent is not None:
            out += ":" + format_length(node.length)
        return out

    return emit(root) + ";"


def matrix_csv(names, matrix):
    """CSV distance matrix with a header row; names already sorted."""
    buffer = io.StringIO()
    writer = csv.writer(buffer, lineterminator="\n")
    writer.writerow([""] + list(names))
    for name, row in zip(names, matrix):
        writer.writerow([name] + [format_length(value) for value in row])
    return buffer.getvalue()
