# tbr — transactional batch rename

A small Go (standard library only) tool that renames a batch of files inside
one directory: preview a JSON plan first, then execute it transactionally.

## Usage

```sh
# Preview: validate the mapping and print a JSON plan (no files changed).
go run . preview -dir /path/to/dir old1=new1 old2=new2 > plan.json

# Execute: re-verify sources against the plan, then rename.
go run . execute -plan plan.json

# Testing aid: inject a failure before the Nth rename step.
go run . execute -plan plan.json -fail-before 4
```

## Guarantees and scope

- **Single directory, regular files only.** Mappings use plain base names;
  path separators and `..` are rejected, symlinks are not followed.
- **Uniqueness is case-insensitive** for both sources and targets. A target
  that already exists is accepted only if it is itself a source in the same
  batch (swaps and cycles).
- **Preview never modifies files.** The plan records old name, new name and
  the SHA-256 of each source.
- **Execute re-verifies before touching anything.** If a source changed or a
  new conflict appeared since preview, the whole batch is refused; the plan
  is never silently regenerated.
- **Two-phase rename.** Every source first moves to an exclusive random temp
  name (`.tbr-tmp-*`), then temps move to their final targets. Swap cycles
  and case-only renames are safe, and no step can overwrite an unrelated
  file. On success the directory contains no leftover temp files and file
  bytes are unchanged.
- **Failure handling.** On any failed step (including the injected one),
  completed steps are rolled back in reverse order. If a rollback rename
  itself fails, the file is preserved at its current location and reported
  under `content preserved at:` — partial recovery is never reported as
  success and temp content is never deleted.
- **Assumptions.** No other writers touch the directory during execution;
  crash recovery and cross-directory moves are out of scope.

## Tests

```sh
go test ./...
```

Covers swap cycles, case-only renames, source changes after preview,
injected-failure rollback, and mapping validation.
