# PRD-28 - Launch-Method-Aware Prefix Initialization

**Status:** `[x] Complete`
**Priority:** High
**Size:** S
**Tags:** `bugfix`, `command`, `prefix`
**Created:** 2026-04-27

---

## 1. Problem & Context

* **Current State:** `InitializePrefix` always uses the Proton script (`proton run cmd /c echo ...`) to create a new Wine prefix, regardless of the `launch_method` set in `game.json`.
* **Impact/Need:** For `launch_method: "direct"`, the Proton script is unnecessary overhead and fails entirely for custom Wine builds (e.g. from the wine-builder) that don't include a `proton` wrapper script. Users expect `direct` to be the lighter path — it should use `wine64 wineboot --init` from the Proton/Wine distribution, matching what `direct` does at runtime.

## 2. Proposed Solution

Branch prefix initialization on `launch_method`:

* `direct` → `initializePrefixWithProtonWine`: runs `wine64 wineboot --init` from the Proton build's bin directory. No Proton script required.
* `container` / `umu` → `initializePrefixWithProtonScript`: current behaviour (Proton script + `restructureProtonPrefix`).
* `system` → unchanged (already handled separately).

## 3. Scope & Design Details

### `internal/command/command.go`

* **`InitializePrefix`:** After the `"system"` guard, add `if appCfg.LaunchMethod == "direct"` branch that calls `initializePrefixWithProtonWine`.
* **`initializePrefixWithProtonWine(absPrefix, prefixPath string, ...)`:** Finds `wine64`/`wine` via `getWineExecutablePath`, sets up PATH + LD_LIBRARY_PATH from `protonVersionInfo.LDLibraryPathComponents`, runs `wineboot --init`, then opens explorer via `RunDirectly`.
* **`initializePrefixWithProtonScript(absPrefix, prefixPath string, ...)`:** Extracts the existing Proton-script path from `InitializePrefix` — opens explorer via `RunInContainer`.

## 4. Execution & Milestones

- [x] Write failing test: `direct` method does NOT require the `proton` script; `container` method DOES
- [x] Implement `initializePrefixWithProtonWine`
- [x] Extract `initializePrefixWithProtonScript`
- [x] Refactor `InitializePrefix` to dispatch on `launch_method`
- [x] All tests pass (`go test ./...`)
- [x] README updated (no user-visible behaviour change, no doc update needed)
- [x] PRD moved to done/

## 5. Technical Notes

### Key Files
* `internal/command/command.go` — `InitializePrefix`, new helpers
* `internal/command/prefix_test.go` — new tests for dispatch logic
