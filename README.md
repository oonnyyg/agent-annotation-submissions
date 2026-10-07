# copybook-record-codec

A Go tool (standard library only, no COBOL compiler required) that converts
fixed-length binary records to and from JSONL using a strict subset of COBOL
copybook record definitions. Field layouts and numeric precision are fully
verifiable.

## Usage

```
cbc layout  -copybook rec.cpy                 # print compiled layout as JSON
cbc to-json -copybook rec.cpy -in data.bin -out data.jsonl
cbc to-bin  -copybook rec.cpy -in data.jsonl -out data.bin
```

The binary file is a plain concatenation of equal-length records with no
separators. A trailing partial record is a hard error. Output is written
atomically: any failing record aborts the whole conversion and no partial
output file is published. Input files are never modified.

## Supported copybook subset

- Free-format source, one top-level `01` record per file. Entries end with
  `.`; `*` comment lines are skipped.
- Group items at levels 01–49 with arbitrary nesting. Every item must be
  named (no FILLER). Duplicate sibling names and illegal levels are
  reported with their source line.
- Fixed `OCCURS n TIMES` on groups and elementary items.
- Pictures:
  - `PIC X(n)` — ASCII text.
  - `PIC 9(n)` / `PIC 9(n)V9(m)` — unsigned DISPLAY (zoned) decimal with
    optional implied decimal places.
  - `PIC S9(n)V9(m) COMP-3` — signed packed decimal (also accepts
    `PACKED-DECIMAL` / `COMPUTATIONAL-3`).

Explicitly rejected (with source line): `REDEFINES`, `OCCURS ... DEPENDING
ON`, levels 66/77/88, signed DISPLAY fields, unsigned COMP-3 fields, and
any other usage clause.

## Layout verification

`cbc layout` compiles the copybook and prints the total record length plus
every group and field with its byte offset, element length, OCCURS count,
digits and scale as JSON. Groups are never collapsed into scalars; nested
paths use dotted names (e.g. `ORDER-RECORD.ORDER-LINES.UNIT-PRICE`).

## JSONL representation and round-trip semantics

- Each line is one record. JSON objects mirror the group nesting; arrays
  must match OCCURS lengths exactly. Unknown or missing keys are errors.
- All numbers travel as decimal strings (e.g. `"-1234.56"`) — no floats,
  ever. Values that exceed the declared integer digits or decimal places
  are rejected, never truncated.
- Text is ASCII only. Encoding right-pads short values with spaces and
  rejects overlong values and non-printable/non-ASCII bytes. Decoding
  preserves padding spaces, so a binary → JSONL → binary round trip is
  byte-identical.
- COMP-3 accepts only sign nibble `C` (positive) and `D` (negative); every
  digit nibble must be 0–9, and the pad nibble produced by an even digit
  count must be zero. Zero is always encoded with the positive `C` sign.
- Decode errors report the 1-based record number and the dotted field path
  (with array index), e.g. `record 2: field REC.BALANCE: invalid comp-3
  sign nibble 0xF (want C or D)`.

## Development

```
go test ./...
```

Tests cover nested repeating groups, negative scaled decimals, COMP-3
nibble validation, text padding rules, precision rejection, truncated
input files, and atomic output.
