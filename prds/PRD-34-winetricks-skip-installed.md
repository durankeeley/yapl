# PRD-34 - Winetricks: Skip Already-Installed Packages

**Status:** `[ ] In Progress`
**Priority:** Medium
**Size:** S
**Sprint:** 2
**Tags:** `winetricks`, `performance`
**Created:** 2026-05-09

---

## 1. Problem & Context

* **Current State:** `EnsureWinetricks` runs winetricks for every package listed in `game.json` on every `setup` and `run` invocation, regardless of whether the package was already installed.
* **Impact/Need:** Winetricks re-verifies or re-installs packages each time, adding several seconds to launch time. For example, a prefix with `dinput8` already installed (recorded in `prefix/winetricks.log`) still runs winetricks on every launch.

## 2. Proposed Solution

Before invoking winetricks, read `prefix/winetricks.log` (one package name per line) and filter out any packages already recorded there. If all requested packages are already installed, skip the winetricks invocation entirely.

## 3. Scope & Design Details

### `EnsureWinetricks` filter logic
* **Behavior/Rule:** Read `<prefixPath>/winetricks.log`; parse one token per line. Filter the requested package list to only those not present in the log. If the filtered list is empty, return nil without calling winetricks.
* **Specifics:** Missing log file is not an error — treat it as "nothing installed yet".

### No changes to winetricks invocation
* **Behavior/Rule:** The actual winetricks call (`--unattended <pkgs...>`) is unchanged. Winetricks itself appends to the log after a successful install, so YAPL does not need to write to it.

## 4. Execution & Milestones

- [x] Write failing tests for `filterUninstalledPackages` (log missing, all installed, partial, none installed)
- [x] Implement `filterUninstalledPackages` helper
- [x] Update `EnsureWinetricks` to call the filter before invoking winetricks
- [x] All tests passing

## 5. Technical Notes

### Key Files & Systems Targeted
* `internal/dependency/dependency.go` — `EnsureWinetricks` and new `filterUninstalledPackages` helper
* `internal/dependency/dependency_test.go` — new table-driven tests

### Risks & Considerations
* **Log format:** The log has one package name per line with no extra formatting; simple `strings.Split` on newlines is sufficient.
* **Winetricks writes the log:** YAPL must not write to `winetricks.log` itself — that would diverge from what winetricks actually records.
