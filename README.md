# torrent-piece-auditor

A read-only torrent content verifier. It maps every piece hash in a v1
torrent's metainfo onto the local files under a content root and reports
exactly which file ranges each corrupt or unverifiable piece covers.

It never connects to peers, never downloads data, and never modifies the
checked files.

## Usage

```
go run ./cmd/torrentcheck <torrent-file> <content-root>
```

Exit codes:

- `0` — every piece verified OK.
- `1` — the report contains corrupt pieces, unverifiable pieces, missing
  files, or length mismatches.
- `2` — usage error, unreadable/invalid torrent, or unsafe paths.

## Supported scope

- BitTorrent v1 metainfo only (single-file and multi-file layouts).
- Bencode parsing records raw byte offsets; truncated input, duplicate
  dictionary keys, and non-canonical integers (leading zeros, `-0`,
  non-digits, overflow) are rejected. Out-of-order dictionary keys are
  accepted but reported under `warnings`.
- The info hash is SHA-1 over the raw `info` dictionary byte slice from
  the original file — never over a re-encoded structure.
- Structural validation: positive `piece length`, `pieces` length a
  multiple of 20, hash count exactly matching `ceil(total/pieceLength)`,
  non-negative file lengths. Empty files consume no piece data; the final
  piece may be short.
- Path safety: absolute paths, drive-relative components, `.`/`..`
  segments, and embedded separators are rejected. Distinct paths that
  collide case-insensitively (Windows) are rejected. Before any content
  file is opened, every declared path is resolved under the content root
  and symlinks along existing ancestors must not escape the root.
- Verification is streaming with a fixed-size buffer; memory use does not
  grow with total content size. Missing or truncated files mark the
  affected pieces `unverifiable`, and later pieces are still located by
  declared lengths — the stream never shifts out of alignment.

## Report format

JSON object on stdout:

- `info_hash`, `name`, `piece_length`, `piece_count`, `total_size` —
  metainfo summary (`info_hash` is hex SHA-1 of the raw info bytes).
- `warnings` — non-fatal issues such as unsorted dictionary keys.
- `pieces[]` — one entry per piece:
  - `index`, `offset`, `length` — position in the concatenated stream.
  - `status` — `ok`, `corrupt` (hash mismatch), or `unverifiable`
    (covers missing/unreadable data).
  - `sha1` — actual hash, present for verifiable pieces.
  - `files[]` — covered ranges: `path`, `file_offset`, `length`.
- `files[]` — one entry per declared file: `path`, `declared_length`,
  `actual_length` (`null` when missing), and `status` (`ok`, `missing`,
  `length_mismatch`).
- `summary` — counts: `pieces_ok`, `pieces_corrupt`,
  `pieces_unverifiable`, `files_ok`, `files_missing`,
  `files_length_mismatch`.

## Development

```
go test ./...
go vet ./...
```

Layout: `internal/bencode` (offset-preserving parser), `internal/torrent`
(metainfo model + path safety), `internal/audit` (streaming verifier),
`cmd/torrentcheck` (CLI).
