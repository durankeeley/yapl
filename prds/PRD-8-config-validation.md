# PRD-8 - Config Validation Before Run/Setup

**Status:** `[ ] Pending`
**Priority:** High
**Size:** S
**Sprint:** 3 — Validation & Deployment Flexibility
**Tags:** `bugfix`, `config`, `ux`, `validation`
**Created:** 2026-04-27

---

## 1. Problem & Context

YAPL reads `game.json` at startup but does not validate required fields before proceeding. If a user omits `executable` or `proton_version`, the error surfaces deep in `RunDirectly`/`RunInContainer` as a cryptic OS-level failure (e.g., `exec: "": executable file not found`) rather than a clear config error at startup.

* **Current State:** No validation pass between config load and command execution.
* **Impact/Need:** Debugging is hard; users must know the internals to understand what went wrong.

## 2. Proposed Solution

* Add `func (a AppConfig) Validate() error` (or a standalone `validateAppConfig`) in `internal/config/` that checks all required fields.
* Call it in `app.go` immediately after config load, before any command runs.
* Return a clear, human-readable error listing which field is missing and what it should contain.

## 3. Scope & Design Details

### Required field checks
* **Behavior/Rule:** The following fields must be non-empty:
  - `proton_version` — must be a non-empty string
  - `executable` — must be a non-empty string
  - `launch_method` — must be one of `direct`, `container`, `umu`
* **Specifics:** `container` method additionally requires `runtime_version` to be non-empty; validate this too.

### Error format
* **Behavior/Rule:** Return a single `error` that lists all violations, not just the first.
* **Specifics:** Collect errors into a slice, join with `"; "` in the returned error message. Example: `config validation failed: executable is required; launch_method must be one of direct, container, umu`.

### When to validate
* **Behavior/Rule:** Validate after config load in `app.go`, before calling `EnsureAll`, `InitializePrefix`, or any launch method.
* **Specifics:** All four commands (`setup`, `run`, `package`, `unpackage`) should validate. `list` does not require a valid single config, so it is exempt.

## 4. Execution & Milestones

- [ ] Write failing test: `TestValidateAppConfig_ReturnsErrorForMissingExecutable`
- [ ] Write failing test: `TestValidateAppConfig_ReturnsErrorForInvalidLaunchMethod`
- [ ] Write failing test: `TestValidateAppConfig_ReturnsErrorForContainerWithNoRuntime`
- [ ] Write failing test: `TestValidateAppConfig_PassesForValidConfig`
- [ ] Implement `ValidateApp(cfg App) error` in `internal/config/config.go`
- [ ] Call `config.ValidateApp` in `app.go` after config load
- [ ] All tests pass
- [ ] Mark Complete, move to `prds/done/`

## 5. Technical Notes

### Key Files & Systems Targeted
* `internal/config/config.go` — add `ValidateApp`
* `internal/config/config_test.go` — new tests
* `internal/app/app.go` — call `ValidateApp` early in each command path

### Risks & Considerations
* **`package` and `unpackage`:** These commands do not launch the game but still need a valid config to operate on the right directory. Validate fully for consistency.
* **`proton_version: "system"`:** `"system"` is a valid value; do not reject it.
* **Backwards compatibility:** Existing valid configs pass validation unchanged; this only catches configs that would previously fail silently at a later stage.
