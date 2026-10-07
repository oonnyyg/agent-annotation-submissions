# Hierarchical Ignore Auditor

A read-only auditor that walks a directory tree, applies hierarchical
`.gitignore` rules using only the Go standard library, and explains why
each path is kept or excluded. It never reads the Git index, never reads
global/system Git config, and never invokes the `git` command.

## Usage

```
go run . -root <dir>            # human-readable audit report
go run . -root <dir> -json      # JSON report (kept / ignored / pruned / skipped)
go run . -root <dir> -explain <relative/path>
go run . -root <dir> -explain <relative/path> -json
```

All output lists are sorted for stable, diff-friendly reports. The tool
writes nothing to the audited tree.

## Rule semantics

- Rules are read from `.gitignore` files found during the walk; each
  file's rules are relative to the directory containing it.
- `*`, `?`, and `[...]` character classes (including `[!...]` negation
  and ranges like `[a-z]`) match within a single path segment.
- `**` matches zero or more whole path segments in the forms `**/x`,
  `x/**`, `x/**/y`, per Git's specification.
- A leading `/` anchors the pattern to its `.gitignore` directory; a
  trailing `/` restricts the rule to directories. Patterns containing
  no slash match a name at any depth below their directory.
- Blank lines and `#` comments are ignored; `\#` and `\!` match literal
  leading characters; unescaped trailing spaces are stripped (`\ ` keeps
  a literal trailing space).
- `!` negates a rule. Within one file the last matching rule wins, and
  rules from deeper `.gitignore` files override ancestor rules.
- Matching is always case-sensitive, regardless of the host OS.

## Traversal and safety limits

- Every regular file is treated as untracked; Git index state is not
  consulted.
- Once a directory is ignored it is pruned: its contents are not read,
  and negation rules inside it cannot re-include descendants. A child
  path can only reappear after an ancestor directory itself is restored
  by a negation at or above its level.
- Symlinks and Windows directory junctions are never followed; they are
  recorded in the report's `skipped` list. The walk therefore cannot
  escape the root directory through links.
- `-explain` rejects absolute paths and `..` components that would
  escape the root.
- Non-regular, non-directory items (devices, sockets, pipes) are
  skipped and recorded.

## Explanation output

`-explain` distinguishes:

- `direct` — the path itself matched (or was restored by) a rule; the
  report names the rule file and 1-based line number.
- `pruned-dir` — the path is a directory excluded by a rule.
- `ancestor` — the path exists on disk but lies under a pruned
  directory; the report names the blocking ancestor and rule.
- `skipped` — the path is a symlink, junction, or special file.
- `missing` — no such path exists under the root.

## Tests

```
go test ./...
```

Covers glob/character-class matching, `**` forms, anchoring, escaped
`#`/`!`/trailing-space boundaries, same-level last-match-wins, deep
rule overrides, directory restore, pruned-directory blocking, symlink
safety, and explanation output.
