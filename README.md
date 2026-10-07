# Transactional Batch Rename

A small Go tool (standard library only) for renaming batches of regular
files inside one directory: preview a plan first, then execute it
transactionally with verification and rollback.

## Usage

```sh
# Preview: validate and write a JSON plan (never modifies files).
# Note: flags must precede the old=new pairs (standard flag package).
go run ./cmd/batchrename preview -dir DIR -plan plan.json old1=new1 old2=new2

# Execute: re-verify, then apply the plan.
go run ./cmd/batchrename execute -plan plan.json

# Testing hooks: inject a failure before step N / into rollback step N.
go run ./cmd/batchrename execute -plan plan.json -fail-before 3
```

The plan JSON records each entry's old name, new name, and SHA-256 content
digest. Execution prints the step log and the final location of every
source's content as JSON.

## Guarantees and scope

- **Single directory, regular files only.** All names are plain base names;
  path separators and `..` are rejected. Files are checked with `Lstat`
  and anything that is not a regular file (e.g. a symlink) is refused —
  links are never followed.
- **Uniqueness is case-insensitive.** Sources and targets must each be
  unique under case folding. A target that already exists is acceptable
  only if it is itself a source in the same batch (swaps/cycles).
- **Preview is read-only.** `BuildPlan`/`preview` only reads and hashes
  files; nothing is renamed.
- **Execute re-verifies.** Before the first rename, every source is
  re-hashed and compared to the plan, and the directory is re-checked for
  new conflicts. Any drift rejects the whole batch — the plan is never
  silently regenerated.
- **Swaps, cycles, and case-only renames** go through exclusive temporary
  names (`.tbr-<i>-<hash>.tmp`). No step ever overwrites an unrelated
  file: each rename target is either a batch temp name or a validated
  batch target.
- **Rollback on failure.** If any step fails, completed steps are undone
  in reverse order. If a rollback step itself fails, the file is left
  where it still has content, the actual locations are reported in the
  `final` list and `rollbackErrors`, and the result is never reported as
  a successful restore. Nothing is ever deleted; cleanup only concerns
  temp names created by this batch that no longer carry content.
- **Success invariants.** File bytes are unchanged (only `os.Rename` is
  used) and no batch temp files remain in the directory.

## Assumptions / non-goals

- No other writer touches the directory during execution.
- No crash recovery: a process kill mid-batch can leave temp files behind.
- No cross-directory moves.

## Tests

```sh
go test ./...
```

Covers swap cycles, case-only renames, source mutation after preview
(batch refused), failure injection with reverse-order rollback, rollback
failure reporting with content preserved, and validation rules.
