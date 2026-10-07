# hierarchical-ignore-auditor

A read-only auditor that walks a directory tree, applies hierarchical
`.gitignore` rules, and explains — with file and line numbers — why each
path is kept or excluded. It uses only the Go standard library: it never
reads the Git index or global config, and never invokes `git`.

## Usage

```sh
go run ./cmd/ignoreaudit [root]                 # text report
go run ./cmd/ignoreaudit -json [root]           # JSON report
go run ./cmd/ignoreaudit -explain sub/file.log  # explain one path
go run ./cmd/ignoreaudit -json -explain a.log   # JSON explanation
```

The report lists, in stable sorted order:

- **kept**: regular files surviving the filter (all files are treated as
  untracked), with the negation rule that kept them when applicable;
- **pruned**: directories excluded by a rule, whose subtrees are not read;
- **skipped**: symlinks, junctions, and non-regular files.

`-explain` prints `KEPT` or `EXCLUDED` for a root-relative path, the
deciding rule's file and line, and distinguishes a **direct** match from
an **ancestor** block (an ignored parent directory).

## Supported rule syntax

- `*`, `?`, and `[...]` character classes (including `[!...]` negation and
  ranges like `[a-z]`); matching is always case-sensitive.
- `**` path forms: `**/x`, `a/**`, `a/**/b` (zero or more whole segments).
- Leading `/` anchors a pattern to the directory containing the
  `.gitignore`; any other pattern containing `/` is also anchored there.
  Patterns without `/` match the basename at any depth below that
  directory.
- Trailing `/` restricts a pattern to directories.
- Blank lines and `#` comments are ignored; `\#` and `\!` are literal
  leading characters; unescaped trailing spaces are stripped (`foo\ `
  keeps a literal trailing space).
- `!` negation: the last matching pattern within a level wins, and rules
  from deeper `.gitignore` files override ancestor rules.

## Traversal and safety limits

- **Read-only**: no source file is created, modified, or deleted.
- **Pruning**: once a directory is ignored, its subtree is not read, so
  negation rules inside it cannot revive its descendants. Restoring the
  directory itself (e.g. `build/` then `!build/` in the same or a deeper
  reachable file) re-enables normal evaluation of its children.
- **No link following**: symlinks and directory junctions are recorded
  under `skipped` and never followed, so traversal cannot escape the
  root. `-explain` rejects `..` and empty paths.
- **Scope**: only per-directory `.gitignore` files are consulted —
  no `.git/info/exclude`, no global/core excludes, no index state.
  Matching is byte-oriented and case-sensitive on every OS.

## Development

```sh
go test ./...
```

Tests cover multi-level override, directory pruning and restoration,
escape/comment boundaries, symlink safety, and path explanation.
