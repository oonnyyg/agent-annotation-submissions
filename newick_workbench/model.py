"""Tree data model. Edge lengths are exact Fractions."""

from __future__ import annotations

from fractions import Fraction


class Node:
    __slots__ = ("name", "length", "children", "parent")

    def __init__(self, name=None, length=None, children=None):
        self.name = name  # str or None
        self.length = length  # Fraction or None (root only)
        self.children = list(children) if children else []
        self.parent = None
        for child in self.children:
            child.parent = self

    @property
    def is_leaf(self):
        return not self.children

    def add_child(self, child, length):
        child.length = Fraction(length)
        child.parent = self
        self.children.append(child)
        return child

    def iter_nodes(self):
        yield self
        for child in self.children:
            yield from child.iter_nodes()

    def leaves(self):
        return [n for n in self.iter_nodes() if n.is_leaf]

    def min_leaf_name(self):
        return min(leaf.name for leaf in self.leaves())
