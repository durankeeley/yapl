# PRD-11 - Dependency Upgrade Flags for DXVK, VKD3D, and Runtime

**Status:** `[ ] Pending`
**Priority:** Medium
**Size:** S
**Sprint:** 4 — Dependency & Package UX
**Tags:** `feature`, `cli`, `dependencies`, `upgrade`
**Created:** 2026-04-27

---

## 1. Problem & Context

`app.Run()` and `app.Setup()` accept a `--upgrade-proton` flag that forces re-download of the Proton version. No equivalent flag exists for DXVK, VKD3D, or the Steam Linux Runtime. If a user updates their `runner.json` to point to a new DXVK version, the old directory is used from cache because `ensure()` in `dependency.go` only downloads if the directory does not exist.

* **Current State:** Only `--upgrade-proton` exists. DXVK, VKD3D, and runtime upgrades require manually deleting the relevant `dependencies/` subdirectory.
* **Impact/Need:** Updating graphics layers requires undocumented manual steps; easy to get wrong on multiple machines.

## 2. Proposed Solution

* Add `--upgrade-deps` flag that forces re-download of DXVK and VKD3D for the configured versions.
* Add `--upgrade-runtime` flag that forces re-download of the Steam Linux Runtime.
* Add `--upgrade-all` as a convenience flag equivalent to `--upgrade-proton --upgrade-deps --upgrade-runtime`.
* Pass upgrade flags through `EnsureAll` to the relevant `ensure()` calls.

## 3. Scope & Design Details

### `EnsureAll` signature change
* **Behavior/Rule:** Add an `UpgradeOptions` struct (or a boolean bitmask) to `EnsureAll`'s parameters instead of a bare `forceUpgrade bool`.
* **Specifics:**
  ```go
  type UpgradeOptions struct {
      Proton  bool
      Deps    bool
      Runtime bool
  }
  ```
  Update the existing `forceUpgrade bool` parameter to `opts UpgradeOptions` and update callers.

### `ensure()` force path
* **Behavior/Rule:** When `forceUpgrade` is true for a dependency, delete the existing directory before downloading, matching the existing Proton behaviour.
* **Specifics:** `os.RemoveAll(depPath)` before `ar.Extract`.

### `EnsureRuntime` upgrade
* **Behavior/Rule:** When `opts.Runtime` is true, skip the version-match check and re-download unconditionally.
* **Specifics:** `runtime.go` `EnsureRuntime` receives a `forceUpgrade bool` parameter; pass `opts.Runtime` through.

### CLI flags
* **Behavior/Rule:** `--upgrade-deps`, `--upgrade-runtime`, `--upgrade-all` added alongside existing `--upgrade-proton`.
* **Specifics:** `--upgrade-all` sets all three booleans to `true` before passing to `EnsureAll`.

## 4. Execution & Milestones

- [ ] Write failing test: `TestEnsure_ForceUpgradeDeletesAndRedownloads`
- [ ] Write failing test: `TestEnsureAll_UpgradeOptsPropagatesToDeps`
- [ ] Introduce `UpgradeOptions` struct in `dependency.go`
- [ ] Update `EnsureAll` signature and all callers
- [ ] Update `ensure()` to accept and honour a force flag
- [ ] Update `EnsureRuntime` signature to accept a force flag
- [ ] Add CLI flags in `main.go`
- [ ] All tests pass
- [ ] Mark Complete, move to `prds/done/`

## 5. Technical Notes

### Key Files & Systems Targeted
* `internal/dependency/dependency.go` — `UpgradeOptions` struct, updated `EnsureAll`, `ensure`
* `internal/dependency/runtime.go` — `EnsureRuntime` force flag
* `internal/app/app.go` — pass `UpgradeOptions` to `EnsureAll`
* `cmd/yapl/main.go` — new flags
* `internal/dependency/dependency_test.go` — new tests

### Risks & Considerations
* **Signature change to `EnsureAll`:** This is an internal API only called from `app.go`; no external callers. Change is safe.
* **`--upgrade-all` interaction:** If `--upgrade-all` is passed alongside individual flags, the result is the same (all true). No conflict.
