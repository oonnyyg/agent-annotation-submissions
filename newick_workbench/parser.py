"""Newick parser: multifurcation, quoted labels, bracket comments.

Lengths are parsed as exact Fractions so distance ties are exact.
"""

from __future__ import annotations

from fractions import Fraction

from .model import Node


class ParseError(ValueError):
    pass


_DELIMS = set("(),:;[]' \t\r\n")


class _Parser:
    def __init__(self, text):
        self.text = text
        self.pos = 0

    def error(self, msg):
        raise ParseError(f"{msg} (at offset {self.pos})")

    def skip_trivia(self):
        while self.pos < len(self.text):
            ch = self.text[self.pos]
            if ch in " \t\r\n":
                self.pos += 1
            elif ch == "[":
                end = self.text.find("]", self.pos + 1)
                if end < 0:
                    self.error("unterminated comment")
                self.pos = end + 1
            else:
                return

    def peek(self):
        return self.text[self.pos] if self.pos < len(self.text) else ""

    def parse_label(self):
        self.skip_trivia()
        if self.peek() == "'":
            self.pos += 1
            parts = []
            while True:
                if self.pos >= len(self.text):
                    self.error("unterminated quoted label")
                ch = self.text[self.pos]
                if ch == "'":
                    if self.text[self.pos + 1 : self.pos + 2] == "'":
                        parts.append("'")
                        self.pos += 2
                    else:
                        self.pos += 1
                        return "".join(parts)
                else:
                    parts.append(ch)
                    self.pos += 1
        start = self.pos
        while self.pos < len(self.text) and self.text[self.pos] not in _DELIMS:
            self.pos += 1
        raw = self.text[start : self.pos]
        return raw.replace("_", " ") if raw else None

    def parse_length(self):
        self.skip_trivia()
        if self.peek() != ":":
            return None
        self.pos += 1
        self.skip_trivia()
        start = self.pos
        while self.pos < len(self.text) and self.text[self.pos] not in _DELIMS:
            self.pos += 1
        token = self.text[start : self.pos]
        if not token:
            self.error("missing branch length after ':'")
        try:
            value = Fraction(token)
        except (ValueError, ZeroDivisionError):
            self.error(f"invalid branch length {token!r}")
        if value < 0:
            self.error(f"negative branch length {token!r}")
        return value

    def parse_subtree(self):
        self.skip_trivia()
        if self.peek() == "(":
            self.pos += 1
            node = Node()
            while True:
                child = self.parse_subtree()
                child.parent = node
                node.children.append(child)
                self.skip_trivia()
                ch = self.peek()
                if ch == ",":
                    self.pos += 1
                elif ch == ")":
                    self.pos += 1
                    break
                else:
                    self.error("expected ',' or ')'")
            node.name = self.parse_label()
        else:
            node = Node(name=self.parse_label())
            if node.name is None:
                self.error("expected a subtree or leaf label")
        node.length = self.parse_length()
        return node


def parse_tree(text):
    """Parse a single Newick tree, validating the workbench constraints."""
    parser = _Parser(text)
    parser.skip_trivia()
    root = parser.parse_subtree()
    parser.skip_trivia()
    if parser.peek() != ";":
        parser.error("expected ';' after tree")
    parser.pos += 1
    parser.skip_trivia()
    if parser.pos != len(parser.text):
        parser.error("unexpected content after ';' (expected a single tree)")
    _validate(root)
    return root


def _validate(root):
    if root.length is not None and root.length != 0:
        raise ParseError("root branch length must be omitted or zero")
    root.length = None
    leaf_names = []
    for node in root.iter_nodes():
        if node is root:
            continue
        if node.length is None:
            raise ParseError("every non-root edge must have a length")
        if node.is_leaf:
            if not node.name:
                raise ParseError("leaf labels must be non-empty")
            leaf_names.append(node.name)
    if len(set(leaf_names)) != len(leaf_names):
        dupes = sorted({n for n in leaf_names if leaf_names.count(n) > 1})
        raise ParseError(f"duplicate leaf labels: {', '.join(dupes)}")
