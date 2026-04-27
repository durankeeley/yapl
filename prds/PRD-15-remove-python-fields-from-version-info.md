# PRD-15 - Remove Unused PythonHome/PythonPath Fields from VersionInfo

**Status:** `[ ] Pending`
**Priority:** Low
**Size:** S
**Sprint:** Backlog
**Tags:** `refactor`, `config`, `dead-code`
**Created:** 2026-04-27

---

## 1. Problem & Context

`config.VersionInfo` declares `PythonHome` and `PythonPath` fields that are never read anywhere in the codebase — not in `command.go`, `dependency.go`, `app.go`, or anywhere else. They appear in serialised JSON if a user adds them, but they silently do nothing.

* **Current State:** `PythonHome string` and `PythonPath string` are defined in `VersionInfo` at `internal/config/config.go` but are never referenced outside of that struct.
* **Impact/Need:** Users who set these fields get no effect and no warning. The struct is misleading. Any future refactor may accidentally rely on these fields existing.

## 2. Proposed Solution

* Remove `PythonHome` and `PythonPath` from `VersionInfo`.
* If there is a legitimate future use case for Python path configuration, it belongs in a dedicated per-game config field, not in the version info catalogue.
* Grep all usages to confirm nothing reads these fields before removal.

## 3. Scope & Design Details

### Struct cleanup
* **Behavior/Rule:** Delete `PythonHome string \`json:"python_home"\`` and `PythonPath string \`json:"python_path"\`` from `config.VersionInfo`.
* **Specifics:** JSON deserialisation ignores unknown fields by default in Go, so any existing `runner.json` with these fields will continue to parse without error — they will simply be ignored.

### Verification
* **Behavior/Rule:** After removal, `grep -r "PythonHome\|PythonPath" internal/` must return zero results.
* **Specifics:** Run as part of the test step to confirm.

## 4. Execution & Milestones

- [ ] Write failing test: `TestVersionInfo_DoesNotContainPythonFields` (compile-time check via struct literal — any unknown field causes a compile error)
- [ ] Remove `PythonHome` and `PythonPath` from `config.VersionInfo`
- [ ] Run `grep -r "PythonHome\|PythonPath" .` — confirm zero hits
- [ ] All tests pass
- [ ] Mark Complete, move to `prds/done/`

## 5. Technical Notes

### Key Files & Systems Targeted
* `internal/config/config.go` — remove two fields from `VersionInfo`
* `internal/config/config_test.go` — confirm struct shape in tests

### Risks & Considerations
* **Existing runner.json files:** Users who have `python_home` or `python_path` in their `runner.json` will see those keys silently ignored after this change — the same behaviour as before, since nothing ever read them.
* **No API breakage:** `VersionInfo` is internal only; no external callers.
