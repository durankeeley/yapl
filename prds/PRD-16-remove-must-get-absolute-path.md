# PRD-16 - Remove MustGetAbsolutePath and Eliminate log.Fatalf from internal/

**Status:** `[ ] Pending`
**Priority:** Medium
**Size:** S
**Sprint:** Backlog
**Tags:** `refactor`, `error-handling`, `coding-standards`
**Created:** 2026-04-27

---

## 1. Problem & Context

`internal/fs/fs.go` contains `MustGetAbsolutePath()` which calls `log.Fatalf` — a coding standard violation. Library packages in `internal/` must never call fatal functions; only `main.go` is permitted to terminate the process. PRD-1 added `GetAbsolutePath` as the proper replacement, but `MustGetAbsolutePath` was left in place marked as deprecated. Deprecated code that calls `log.Fatalf` is a hidden reliability hazard: if any future code accidentally calls it, the process dies with no chance for the caller to recover or clean up.

* **Current State:** `MustGetAbsolutePath` exists at `internal/fs/fs.go:21`, calls `log.Fatalf`, is marked deprecated, and is not called anywhere in production code.
* **Impact/Need:** Violates coding standards; creates a footgun for future contributors.

## 2. Proposed Solution

* Delete `MustGetAbsolutePath` entirely.
* Confirm with grep that no production or test code calls it.
* If any caller is found, update it to use `GetAbsolutePath` with proper error handling.

## 3. Scope & Design Details

### Deletion
* **Behavior/Rule:** Remove the entire `MustGetAbsolutePath` function from `fs.go`.
* **Specifics:** The function body calls `log.Fatalf`; its removal means no `log.Fatalf` remains anywhere in `internal/`.

### Verification
* **Behavior/Rule:** `grep -r "log\.Fatalf\|MustGetAbsolutePath" internal/` must return zero results after this change.
* **Specifics:** Add this as an assertion in the PR checklist (not a runtime test, but a CI grep step).

## 4. Execution & Milestones

- [ ] Write failing test: `TestGetAbsolutePath_ReturnsErrorOnEmptyInput` (ensures `GetAbsolutePath` is the authoritative function and behaves correctly)
- [ ] Delete `MustGetAbsolutePath` from `internal/fs/fs.go`
- [ ] Grep confirms zero `log.Fatalf` and zero `MustGetAbsolutePath` references in `internal/`
- [ ] All tests pass
- [ ] Mark Complete, move to `prds/done/`

## 5. Technical Notes

### Key Files & Systems Targeted
* `internal/fs/fs.go` — delete `MustGetAbsolutePath`
* `internal/fs/fs_test.go` — update if any test referenced the removed function

### Risks & Considerations
* **No callers in production:** Confirmed by audit. Safe to delete.
* **Test files:** Check `fs_test.go` for any reference to `MustGetAbsolutePath` and remove those test cases — they test code that no longer exists.
