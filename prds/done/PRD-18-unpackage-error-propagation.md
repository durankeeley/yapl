# PRD-18 - Fix Silent Error Swallowing in Unpackage

**Status:** `[x] Complete`
**Priority:** High
**Size:** S
**Sprint:** 2 — Reliability
**Tags:** `bugfix`, `archive`, `error-handling`
**Created:** 2026-04-27

---

## 1. Problem & Context

`archive.Unpackage` iterates over a list of archive files and calls `ar.Extract()` for each. When `ar.Extract()` fails, the error is currently logged with `log.Printf` and the function continues, ultimately returning `nil` — signalling success to the caller even though one or more archives failed to extract. This means a partial unpackage (e.g., a corrupted game directory) appears successful.

* **Current State:** Failed `ar.Extract()` calls in the unpackage loop are logged but the error is discarded; `Unpackage` returns `nil`.
* **Impact/Need:** Users see no error when a file fails to unpack. The game directory is silently left in a broken partial state.

## 2. Proposed Solution

* Change the unpackage loop to return the first extraction error immediately, stopping further processing.
* Do not use `log.Printf` for errors inside `Unpackage` — return the error to the caller so `main.go` can log or display it properly.

## 3. Scope & Design Details

### Error propagation
* **Behavior/Rule:** When `ar.Extract()` returns a non-nil error, `Unpackage` returns that error immediately.
* **Specifics:** `return fmt.Errorf("failed to unpackage %s: %w", archivePath, err)` — wraps the underlying error with context about which archive failed.

### Log removal
* **Behavior/Rule:** Remove `log.Printf` from the extraction loop. The caller (`main.go`) is responsible for surfacing errors to the user.
* **Specifics:** `internal/` packages must not use `log.Printf` for errors that should be returned — this is a coding standards violation.

### Partial cleanup
* **Behavior/Rule:** Do not attempt to clean up partially extracted files on failure. The caller (or the user) decides whether to retry or clean up.
* **Specifics:** Document this in a comment: cleanup is the caller's responsibility.

## 4. Execution & Milestones

- [ ] Write failing test: `TestUnpackage_ReturnsErrorWhenExtractionFails`
- [ ] Update `Unpackage` to return extraction errors immediately
- [ ] Remove `log.Printf` from the extraction loop
- [ ] All tests pass
- [ ] Mark Complete, move to `prds/done/`

## 5. Technical Notes

### Key Files & Systems Targeted
* `internal/archive/archive.go` — `Unpackage` function error handling
* `internal/archive/archive_test.go` — new test for error propagation

### Risks & Considerations
* **Caller impact:** `app.go` calls `Unpackage` and currently assumes `nil` on success. After this change it must handle the error. This is a correct change — callers should always check errors.
* **`log.Printf` in `internal/`:** This is a coding standards violation. PRD-16 tracks removing `log.Fatalf`; this PRD handles `log.Printf` in archive.go.
