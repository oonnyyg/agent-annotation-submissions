# git-pack-object-reader

An offline Git pack inspection tool written in Go (standard library only).
It recovers every object from a pack archive, verifies integrity, and writes
a checklist plus the raw object bytes — without invoking `git` and without
touching any repository.

## Usage

```
go run . -pack path/to/pack-xxxx.pack -out output-dir \
    [-max-object-size N] [-max-delta-depth N]
```

- `-pack` — path to a `.pack` file (required).
- `-out` — output directory to create (required). It must not already exist;
  the tool never overwrites. Output is built in a temporary sibling directory
  and renamed into place only after every object and the manifest have been
  written successfully, so a failed run leaves no half-finished export.
- `-max-object-size` — maximum recovered object size in bytes
  (default 1 GiB). Exceeding it fails the run.
- `-max-delta-depth` — maximum delta chain length (default 64). Exceeding it
  fails the run.

The output directory contains:

- `MANIFEST.txt` — one row per pack entry: offset, final object type, entry
  type, recovered size, recovered object identity, and the delta source
  (base offset or base object id) for delta entries.
- `objects/<hex-id>` — the recovered object body as raw bytes, named by its
  Git object identity.

## Format coverage

- SHA-1 repositories, pack format **version 2** only.
- The four plain object types: commit, tree, blob, tag.
- Both delta encodings: `ofs-delta` (base by pack offset) and `ref-delta`
  (base by object identity). Delta bases may themselves be deltas, and a
  `ref-delta` base may appear later in the pack.
- Object identities are computed as `sha1("<type> <size>\0" + body)`; delta
  entries inherit the type of their final base object.

## Validation and limits

The pack is rejected with a diagnostic when:

- the trailing SHA-1 checksum does not match the pack contents;
- the header magic or version is wrong, or the declared entry count does not
  match the parsed entries;
- an entry has an unknown type, a truncated header/zlib stream, or an
  inflated size different from the declared size;
- extra bytes remain after the last declared entry;
- a delta's base is missing (only self-contained packs are accepted; the
  error names the dependent entry), its base-size header disagrees with the
  actual base, a copy instruction reads out of range, an insert is truncated,
  the reserved opcode `0` is used, or the reconstructed size differs from the
  delta's declared result size — no partial object is produced;
- the configured object-size or delta-depth limits are exceeded.

## Limitations

- SHA-256 repositories and pack version 3 are not supported.
- Pack index (`.idx`) files are not read; the pack is scanned sequentially.
- The whole pack and all recovered objects are held in memory.
- Multi-pack archives and thin packs (bases outside the pack) are rejected.

## Development

```
go test ./...
```

Tests build packs synthetically and cover chained deltas, forward
`ref-delta` references, corrupt archives (bad checksum, unknown type,
truncation, trailing bytes, missing bases, invalid delta instructions),
limit enforcement, and export semantics (no overwrite, atomic publish).
