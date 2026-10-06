# image-layer-auditor

Offline analyzer for ordered, uncompressed OCI-style tar layers. It
rebuilds the final filesystem manifest and per-path provenance without
starting a container and without unpacking anything to the host disk.
Only the Go standard library is used.

## Restricted model

- Only regular files, directories and symlinks are supported; every
  other tar entry type is rejected.
- Permissions, ownership, xattrs and hard links are **not** audited.
- Symlink targets are recorded verbatim and never followed.
- Paths are POSIX-normalized (`path.Clean`). Absolute paths,
  backslashes and `..` traversal are rejected. Two members of the same
  layer that normalize to the same path are rejected.

## Layer semantics

- A regular file overwrites any older entry at its path. Replacing a
  directory with a file or symlink removes its old descendants.
- Missing parent directories are created as implicit directories. If a
  parent path already exists as a non-directory, the layer must replace
  it with an explicit directory entry, otherwise the layer is rejected.
- A non-directory member with descendants in the same layer is rejected.
- OCI whiteouts are honored: an empty regular file named `.wh.<name>`
  deletes the sibling path `<name>` and its descendants from lower
  layers; `.wh..wh..opq` clears all lower-layer descendants of its
  directory. Markers never appear in the result and cannot delete
  members added by the same layer.
- Each layer first applies all deletions, then applies regular members
  parent-first, so tar entry order does not affect the final view.
- A layer is fully validated before it is applied. On failure the last
  good state is kept, the failing layer index and error are reported,
  and processing stops; no half-applied state remains.

## Results

- Files record the SHA-256 of their content and the index of the last
  layer that wrote them; symlinks record their raw target.
- Per-layer reports list added, deleted and changed paths. Rewriting a
  file with identical content only advances its source layer and is not
  counted as a content change.

## Usage

`layers.json` is a JSON array of layer tar paths, base layer first.

```sh
go build ./cmd/auditor
auditor manifest layers.json          # sorted JSON manifest
auditor report  layers.json           # per-layer add/delete/change
auditor history layers.json app/run   # event history of one path
```

All commands print a JSON object with a `result` field; when a layer
fails, `failedLayer` and `error` fields are added and the exit code is 1.

## Tests

```sh
go test ./...
```

Covers opaque directories, whiteout deletes, directory-to-file type
replacement, symlink parent paths, identical-content rewrites, invalid
path rejection and atomic failure handling.
