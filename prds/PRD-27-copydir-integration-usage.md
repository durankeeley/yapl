# PRD-27 - Wire CopyDir into Production Code (Bundle and Prefix Migration)

**Status:** `[ ] Pending`
**Priority:** Low
**Size:** S
**Sprint:** Backlog
**Tags:** `refactor`, `fs`, `dead-code`
**Created:** 2026-04-27

---

## 1. Problem & Context

`internal/fs/fs.go` exports `CopyDir(src, dst string) error` which correctly handles symlinks using `os.Lstat`. It is tested in `fs_test.go` but is never called from any production code. It was written in anticipation of being needed, but nothing in `app.go`, `dependency.go`, or `command.go` currently calls it.

The two places that should use it:
1. PRD-14 (bundle dependencies for offline packaging) — copies Proton and dependency directories into the archive staging area.
2. PRD-23 (wine builder integration) — copies wine builder output into the `proton/` directory.

Without wiring `CopyDir` into production code, it is dead code: tested but unreachable, and will be deleted by any future "remove unused exports" cleanup pass.

* **Current State:** `CopyDir` exported but never called in production.
* **Impact/Need:** Dead code; risk of future deletion; PRD-14 and PRD-23 will need to rediscover and re-test the same logic.

## 2. Proposed Solution

* This PRD is a **tracking PRD** — it does not add new logic. It ensures that when PRD-14 and PRD-23 are implemented, they wire `fs.CopyDir` into their respective code paths rather than reimplementing directory copying.
* Add a compile-time usage in the interim: a small internal helper in `internal/app/app.go` that calls `fs.CopyDir` in the `Package` path so the function is not dead code even before PRD-14 is complete.

## 3. Scope & Design Details

### Interim usage
* **Behavior/Rule:** In `app.Package`, when copying game files to a staging directory before archiving, use `fs.CopyDir(gamePath, stagePath)` instead of archiving the source directory directly.
* **Specifics:** This makes the existing package flow use `CopyDir`, ensures the function is exercised in production, and prepares the structure for PRD-14's bundle staging.

### PRD-14 and PRD-23 integration point
* **Behavior/Rule:** Document in this PRD that PRD-14's bundle staging must call `fs.CopyDir` for each dependency directory being bundled.
* **Specifics:** No code change in this PRD beyond the interim `app.Package` wiring.

## 4. Execution & Milestones

- [ ] Confirm `CopyDir` is not called from any production code (grep)
- [ ] Wire `fs.CopyDir` into `app.Package` staging step
- [ ] Write test: `TestPackage_PreservesSymlinksInGameDirectory`
- [ ] All tests pass
- [ ] Mark Complete, move to `prds/done/`

## 5. Technical Notes

### Key Files & Systems Targeted
* `internal/app/app.go` — use `fs.CopyDir` in `Package`
* `internal/fs/fs.go` — no changes (function already correct)

### Risks & Considerations
* **Staging overhead:** Copying a multi-GB game directory to a staging area before archiving uses extra disk space. Ensure the staging dir is in `t.TempDir()` (cleaned up) in tests, and use a temp path under the deployment root in production.
* **Symlink preservation:** This is the key benefit — `fs.CopyDir` correctly handles the `pfx → .` symlink in Wine prefixes. A naive `filepath.Walk` + copy would follow the symlink and recurse infinitely.
