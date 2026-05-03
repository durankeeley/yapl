# PRD-24 - Propagate Runtime Update Check Errors Instead of Swallowing Them

**Status:** `[ ] Pending`
**Priority:** Medium
**Size:** S
**Sprint:** 2 — Reliability
**Tags:** `bugfix`, `runtime`, `error-handling`
**Created:** 2026-04-27

---

## 1. Problem & Context

`internal/dependency/runtime.go`'s `EnsureRuntime` calls `runtimeNeedsUpdate()` to check whether the local runtime is out of date. When `runtimeNeedsUpdate()` returns an error (e.g., network failure, malformed version file), the current code logs the error with `log.Printf` and then continues as if the runtime does not need updating — silently using whatever is cached.

* **Current State:** `runtimeNeedsUpdate()` errors are swallowed: logged and then treated as "no update needed".
* **Impact/Need:** A network timeout or corrupt version file means the runtime silently stays outdated. The user has no idea the check failed. `log.Printf` in an internal package also violates coding standards.

## 2. Proposed Solution

* When `runtimeNeedsUpdate()` returns an error, propagate it to the caller rather than swallowing it.
* The caller (`app.Setup`, `app.Run`) decides whether a version-check failure is fatal (strict mode) or should be treated as a warning (lenient mode with explicit user output).
* For now: return the error; `app.go` prints a warning and continues (the runtime is likely still usable even if the update check failed).

## 3. Scope & Design Details

### Error propagation in EnsureRuntime
* **Behavior/Rule:** Change:
  ```go
  needsUpdate, err := runtimeNeedsUpdate(...)
  if err != nil {
      log.Printf("warning: could not check runtime version: %v", err)
      needsUpdate = false
  }
  ```
  To:
  ```go
  needsUpdate, err := runtimeNeedsUpdate(...)
  if err != nil {
      return fmt.Errorf("runtime version check failed: %w", err)
  }
  ```
* **Specifics:** Remove the `log.Printf`.

### Caller handling in app.go
* **Behavior/Rule:** In `app.Setup`/`app.Run`, catch the "runtime version check failed" error, print a user-facing warning to stderr, and continue (do not abort).
* **Specifics:** Check for the error by wrapping message prefix or use `errors.As` with a typed error.
* The runtime is still present on disk from a previous download; the game can still launch. Only a missing runtime should be fatal.

## 4. Execution & Milestones

- [ ] Write failing test: `TestEnsureRuntime_ReturnsErrorWhenVersionCheckFails`
- [ ] Update `EnsureRuntime` to propagate the version check error
- [ ] Remove `log.Printf` from the error path
- [ ] Update `app.go` to print a stderr warning and continue when this specific error is returned
- [ ] All tests pass
- [ ] Mark Complete, move to `prds/done/`

## 5. Technical Notes

### Key Files & Systems Targeted
* `internal/dependency/runtime.go` — error propagation in `EnsureRuntime`
* `internal/dependency/runtime_test.go` — new test
* `internal/app/app.go` — lenient error handling for runtime version check

### Risks & Considerations
* **Offline environments:** This is especially important for offline LAN scenarios. The version check will always fail without network; the lenient handler in `app.go` ensures the game still launches.
* **`log.Printf` removal:** Consistent with PRD-16 and PRD-18 — internal packages must return errors, not log them.
