"""Newick distance workbench.

Parse a single Newick tree with branch lengths, prune to chosen leaves,
midpoint-root, and export deterministic Newick plus a CSV distance matrix.
Standard library only; sample distances are preserved exactly.
"""

from __future__ import annotations

import argparse
import csv
import math
import sys


class NewickError(ValueError):
    """Raised when the input Newick string is invalid."""


class Node:
    __slots__ = ("label", "length", "children", "parent")

    def __init__(self, label=None, length=None):
        self.label = label
        self.length = length
        self.children = []
        self.parent = None

    @property
    def is_leaf(self):
        return not self.children


def _tokenize(text):
    i = 0
    n = len(text)
    while i < n:
        c = text[i]
        if c.isspace():
            i += 1
            continue
        if c == "[":
            j = text.find("]", i + 1)
            if j == -1:
                raise NewickError("unterminated '[' comment")
            i = j + 1
            continue
        if c in "(),:;":
            yield (c, c)
            i += 1
            continue
        if c == "'":
            i += 1
            buf = []
            while True:
                if i >= n:
                    raise NewickError("unterminated quoted label")
                if text[i] == "'":
                    if i + 1 < n and text[i + 1] == "'":
                        buf.append("'")
                        i += 2
                        continue
                    i += 1
                    break
                buf.append(text[i])
                i += 1
            yield ("LABEL", "".join(buf))
            continue
        start = i
        while i < n and not text[i].isspace() and text[i] not in "(),:;['":
            i += 1
        if start == i:
            raise NewickError("unexpected character %r at offset %d" % (text[i], i))
        yield ("LABEL", text[start:i].replace("_", " "))


class _Parser:
    def __init__(self, text):
        self.tokens = list(_tokenize(text))
        self.pos = 0

    def peek(self):
        if self.pos < len(self.tokens):
            return self.tokens[self.pos]
        return (None, None)

    def next(self):
        tok = self.peek()
        self.pos += 1
        return tok

    def parse(self):
        root = self.subtree()
        kind, _ = self.next()
        if kind != ";":
            raise NewickError("expected ';' at end of tree")
        if self.pos != len(self.tokens):
            raise NewickError("unexpected content after ';'")
        return root

    def subtree(self):
        kind, _ = self.peek()
        if kind == "(":
            self.next()
            node = Node()
            while True:
                child = self.subtree()
                child.parent = node
                node.children.append(child)
                kind, _ = self.next()
                if kind == ",":
                    continue
                if kind == ")":
                    break
                raise NewickError("expected ',' or ')' in child list")
        else:
            node = Node()
        kind, value = self.peek()
        if kind == "LABEL":
            self.next()
            node.label = value
        kind, _ = self.peek()
        if kind == ":":
            self.next()
            kind, value = self.next()
            if kind != "LABEL":
                raise NewickError("expected a number after ':'")
            try:
                node.length = float(value)
            except ValueError:
                raise NewickError("invalid branch length %r" % value)
        return node


def parse_newick(text):
    """Parse a single Newick tree and validate it."""
    root = _Parser(text).parse()
    _validate(root)
    return root


def _validate(root):
    if root.length is not None and root.length != 0:
        raise NewickError("root branch length must be omitted or zero")
    root.length = None
    if root.is_leaf:
        raise NewickError("tree must contain at least one internal node")
    names = {}
    for node in _iter_nodes(root):
        if node.is_leaf:
            if not node.label:
                raise NewickError("leaf label must be non-empty")
            if node.label in names:
                raise NewickError("duplicate leaf label %r" % node.label)
            names[node.label] = node
        if node is not root:
            if node.length is None:
                raise NewickError("every non-root edge must have a length")
            if not math.isfinite(node.length) or node.length < 0:
                raise NewickError("branch lengths must be finite and non-negative")


def _iter_nodes(root):
    stack = [root]
    while stack:
        node = stack.pop()
        yield node
        stack.extend(node.children)


def leaves_of(root):
    return [n for n in _iter_nodes(root) if n.is_leaf]


def _adjacency(root):
    """Build an undirected adjacency map: node -> [(neighbor, length)]."""
    adj = {n: [] for n in _iter_nodes(root)}
    for node in _iter_nodes(root):
        for child in node.children:
            adj[node].append((child, child.length))
            adj[child].append((node, child.length))
    return adj


def distances_from(adj, start):
    """Distances from one node to every node in the tree (DFS)."""
    dist = {start: 0.0}
    stack = [start]
    while stack:
        node = stack.pop()
        for nbr, length in adj[node]:
            if nbr not in dist:
                dist[nbr] = dist[node] + length
                stack.append(nbr)
    return dist


def distance_matrix(root):
    """Return (names, matrix) with leaf names sorted and pairwise distances."""
    adj = _adjacency(root)
    leaves = sorted(leaves_of(root), key=lambda n: n.label)
    names = [n.label for n in leaves]
    matrix = []
    for leaf in leaves:
        dist = distances_from(adj, leaf)
        matrix.append([dist[other] for other in leaves])
    return names, matrix


def prune(root, keep_names):
    """Prune the tree in place to the given leaf names.

    Returns a sorted list of internal labels removed while compressing
    one-in/one-out nodes; those labels are reported, never reassigned.
    """
    keep = set(keep_names)
    if len(keep) < 2:
        raise NewickError("at least two leaf names must be kept")
    leaves = {n.label: n for n in leaves_of(root)}
    missing = sorted(keep - set(leaves))
    if missing:
        raise NewickError("unknown leaf name(s): %s" % ", ".join(missing))

    def wanted(node):
        if node.is_leaf:
            return node.label in keep
        node.children = [c for c in node.children if wanted(c)]
        return bool(node.children)

    if not wanted(root):
        raise NewickError("nothing kept after pruning")

    removed_labels = []

    def compress(node):
        for child in list(node.children):
            compress(child)
        while len(node.children) == 1 and node.parent is not None:
            child = node.children[0]
            child.length += node.length
            child.parent = node.parent
            idx = node.parent.children.index(node)
            node.parent.children[idx] = child
            if node.label:
                removed_labels.append(node.label)
            node = child  # continue in case parent chain needs re-check
        return node

    compress(root)
    while len(root.children) == 1:
        child = root.children[0]
        if root.label:
            removed_labels.append(root.label)
        child.parent = None
        child.length = None
        root = child
    return root, sorted(removed_labels)


def _path_between(adj, start, goal):
    """Return [(node, edge_len_to_next)] from start to goal (last is 0)."""
    prev = {start: None}
    stack = [start]
    while stack:
        node = stack.pop()
        if node is goal:
            break
        for nbr, length in adj[node]:
            if nbr not in prev:
                prev[nbr] = (node, length)
                stack.append(nbr)
    path = [(goal, 0.0)]
    while prev[goal] is not None:
        node, length = prev[goal]
        path.append((node, length))
        goal = node
    path.reverse()
    return path


def _reroot_at(node):
    """Reorient the tree so that node becomes the root."""
    adj = _adjacency(_root_of(node))
    node.parent = None
    node.length = None
    node.children = []
    stack = [node]
    while stack:
        cur = stack.pop()
        for nbr, length in adj[cur]:
            if nbr is cur.parent:
                continue
            nbr.parent = cur
            nbr.length = length
            nbr.children = []
            cur.children.append(nbr)
            stack.append(nbr)
    return node


def _root_of(node):
    while node.parent is not None:
        node = node.parent
    return node


def midpoint_root(root):
    """Midpoint-root the tree in place and return the new root.

    The farthest leaf pair is chosen; ties go to the lexicographically
    smallest sorted name pair.  The halfway point is located walking from
    the lexicographically smaller leaf, so with consecutive zero-length
    edges the first halfway node reached wins.  If the midpoint is exactly
    a leaf, the leaf keeps its identity and joins the new root with a
    zero-length edge.
    """
    adj = _adjacency(root)
    leaves = sorted(leaves_of(root), key=lambda n: n.label)
    best_pair = None
    best_dist = -1.0
    for i, a in enumerate(leaves):
        dist = distances_from(adj, a)
        for b in leaves[i + 1:]:
            d = dist[b]
            if d > best_dist:
                best_dist = d
                best_pair = (a, b)
    a, b = best_pair
    half = best_dist / 2.0

    path = _path_between(adj, a, b)
    acc = 0.0
    for idx in range(len(path) - 1):
        from_node, edge_len = path[idx]
        to_node = path[idx + 1][0]
        if acc + edge_len < half:
            acc += edge_len
            continue
        # The midpoint lies on this edge or at one of its endpoints.
        offset = half - acc
        if offset == 0.0:
            if from_node.is_leaf:
                return _reroot_at_leaf_midpoint(from_node)
            return _reroot_at(from_node)
        if offset == edge_len:
            if to_node.is_leaf:
                return _reroot_at_leaf_midpoint(to_node)
            return _reroot_at(to_node)
        return _split_edge(from_node, to_node, offset)
    # best_dist == 0: midpoint is the starting leaf itself.
    return _reroot_at_leaf_midpoint(a)


def _reroot_at_leaf_midpoint(leaf):
    """Midpoint is exactly a leaf (only possible in an all-zero tree).

    Re-root at the leaf's parent so the leaf keeps its identity and joins
    the new root through its existing zero-length edge.
    """
    return _reroot_at(leaf.parent)


def _split_edge(from_node, to_node, offset):
    """Split the edge from_node->to_node at distance offset from from_node."""
    new_root = Node()
    if to_node.parent is from_node:
        total = to_node.length
        idx = from_node.children.index(to_node)
        from_node.children[idx] = new_root
        new_root.parent = from_node
        new_root.length = offset
        new_root.children = [to_node]
        to_node.parent = new_root
        to_node.length = total - offset
    else:
        total = from_node.length
        idx = to_node.children.index(from_node)
        to_node.children[idx] = new_root
        new_root.parent = to_node
        new_root.length = total - offset
        new_root.children = [from_node]
        from_node.parent = new_root
        from_node.length = offset
    return _reroot_at(new_root)


def _format_number(x):
    if x == int(x):
        return str(int(x))
    return repr(x)


def _quote_label(label):
    if label and not any(c in label for c in " \t\r\n'(),:;[]"):
        return label.replace(" ", "_")
    return "'" + label.replace("'", "''") + "'"


def _min_leaf_label(node):
    if node.is_leaf:
        return node.label
    return min(_min_leaf_label(c) for c in node.children)


def to_newick(root):
    """Serialize deterministically: subtrees ordered by smallest leaf name."""

    def render(node):
        parts = []
        if node.children:
            ordered = sorted(node.children, key=_min_leaf_label)
            parts.append("(")
            parts.append(",".join(render(c) for c in ordered))
            parts.append(")")
        if node.label:
            parts.append(_quote_label(node.label))
        if node.length is not None:
            parts.append(":" + _format_number(node.length))
        return "".join(parts)

    return render(root) + ";"


def matrix_csv(names, matrix, out):
    writer = csv.writer(out, lineterminator="\n")
    writer.writerow([""] + names)
    for name, row in zip(names, matrix):
        writer.writerow([name] + [_format_number(x) for x in row])


def main(argv=None):
    parser = argparse.ArgumentParser(
        prog="newick-workbench",
        description="Prune a Newick tree to chosen leaves, midpoint-root it, "
        "and export a deterministic Newick tree plus a CSV distance matrix.",
    )
    parser.add_argument("input", help="input Newick file ('-' for stdin)")
    parser.add_argument("--keep", nargs="+", metavar="LEAF",
                        help="leaf names to keep (at least two); default: keep all")
    parser.add_argument("--out-tree", metavar="FILE",
                        help="write the midpoint-rooted Newick tree here")
    parser.add_argument("--out-matrix", metavar="FILE",
                        help="write the CSV leaf-distance matrix here")
    args = parser.parse_args(argv)

    if args.input == "-":
        text = sys.stdin.read()
    else:
        with open(args.input, "r", encoding="utf-8-sig") as fh:
            text = fh.read()
    try:
        root = parse_newick(text)
        if args.keep:
            root, removed = prune(root, args.keep)
            for label in removed:
                print("removed internal label: %s" % label, file=sys.stderr)
        root = midpoint_root(root)
    except NewickError as exc:
        parser.exit(2, "error: %s\n" % exc)

    tree_text = to_newick(root) + "\n"
    if args.out_tree:
        with open(args.out_tree, "w", encoding="utf-8", newline="") as fh:
            fh.write(tree_text)
    else:
        sys.stdout.write(tree_text)

    names, matrix = distance_matrix(root)
    if args.out_matrix:
        with open(args.out_matrix, "w", encoding="utf-8", newline="") as fh:
            matrix_csv(names, matrix, fh)
    else:
        matrix_csv(names, matrix, sys.stdout)


if __name__ == "__main__":
    main()
