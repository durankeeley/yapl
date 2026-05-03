# PRD-12 - Package with Prefix Exclusion Option

**Status:** `[ ] Pending`
**Priority:** Medium
**Size:** S
**Sprint:** 4 — Dependency & Package UX
**Tags:** `feature`, `package`, `disk-space`
**Created:** 2026-04-27

---

## 1. Problem & Context

`yapl package` always includes the full game directory, which contains the Wine prefix. A Wine prefix for a modern game can exceed 2–5 GB of Windows system files. When packaging for distribution, operators often only need the `game.json` config and possibly the game executable, not the full prefix (which can be regenerated with `yapl setup` on the target machine).

* **Current State:** `Package()` in `app.go` calls `archive.Package` on the entire game directory with no exclusion option.
* **Impact/Need:** Packages are unnecessarily large; transferring a 4 GB archive when only a 10 MB config + exe is needed wastes LAN bandwidth and storage.

## 2. Proposed Solution

* Add a `--no-prefix` flag to the `package` command.
* When `--no-prefix` is set, exclude the `prefix/` subdirectory from the archive.
* Print a note to the operator: `Note: prefix excluded; run 'yapl setup' on the target machine before running.`

## 3. Scope & Design Details

### Archive.Package exclusion
* **Behavior/Rule:** `archive.Package` receives an optional `[]string` of directory/file names to exclude relative to the source root.
* **Specifics:** During the `filepath.Walk`, skip any entry whose path starts with `sourceRoot/prefix/`. Compare using `filepath.HasPrefix` equivalent — clean both paths before comparison.

### CLI flag
* **Behavior/Rule:** `--no-prefix` bool flag on the `package` sub-command.
* **Specifics:** Passed through `app.Package(excludePrefix bool)`.

### Archive.Package signature
* **Behavior/Rule:** Add `excludes []string` parameter to `archive.Package(sourceDir, destPath string, format string, excludes []string) error`.
* **Specifics:** `nil` or empty slice means no exclusions — existing behaviour is preserved.

### Operator note
* **Behavior/Rule:** When `--no-prefix` is active, print to stdout after successful package: `Note: Wine prefix excluded. Run 'yapl --game <name> setup' on the target machine before running.`
* **Specifics:** Do not print this note when packaging with the prefix included.

## 4. Execution & Milestones

- [ ] Write failing test: `TestPackage_ExcludesPrefixDirWhenFlagSet`
- [ ] Write failing test: `TestPackage_IncludesPrefixDirByDefault`
- [ ] Update `archive.Package` signature to accept `excludes []string`
- [ ] Implement exclusion logic in `archive.Package`
- [ ] Add `--no-prefix` flag in `main.go`
- [ ] Update `app.Package` to pass exclusion list
- [ ] All tests pass
- [ ] Mark Complete, move to `prds/done/`

## 5. Technical Notes

### Key Files & Systems Targeted
* `internal/archive/archive.go` — `Package` exclusion parameter + logic
* `internal/archive/archive_test.go` — new tests
* `internal/app/app.go` — `Package(excludePrefix bool) error`
* `cmd/yapl/main.go` — `--no-prefix` flag

### Risks & Considerations
* **Backwards compatibility:** All existing callers pass `nil` for `excludes`; behaviour is unchanged.
* **Nested prefix paths:** The prefix directory is always named `prefix/` directly under the game directory. No need for glob patterns; a simple prefix string match is sufficient.
* **User expectation:** Make the note impossible to miss. Without a prefix the game will not run until `setup` completes.
