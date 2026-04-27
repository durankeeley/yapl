# PRD 3 - Prefix Setup Reliability & External Prefix Import

**Status:** `[ ] Pending` | `[ ] In Progress` | `[x] Complete`
**Priority:** High
**Size:** L
**Sprint:** 2
**Tags:** `prefix`, `setup`, `lutris`, `import`, `reliability`
**Created:** 2026-04-27
**GitHub Issue:** N/A

---

## 1. Problem & Context

A code review of `InitializePrefix` and related functions uncovered several bugs that make prefix setup unreliable, especially when importing an existing Wine prefix from Lutris or another launcher.

* **Current State:**
  - The "prefix already exists" check only looks for `system.reg` in the root of the prefix directory. Lutris and some older Proton builds store `system.reg` inside a `pfx/` subdirectory — YAPL misses this and re-initialises, potentially overwriting or conflicting with the existing prefix.
  - `ProtonVersion: "system"` is completely broken: when selected, the prefix init block is skipped but the code then tries to launch `explorer.exe` against a prefix that was never created, causing a silent crash.
  - `filepath.Abs` errors are silently discarded in four places in `command.go` (`_, _ := filepath.Abs(...)`). If path resolution fails, `protonBasePath` becomes an empty string and subsequent failures produce cryptic errors with no root cause.
  - `restructureProtonPrefix` reads the prefix root when `pfx/` is already a symlink to `.`, causing it to try to rename every file to itself. While this is harmless (no-op renames), it produces confusing output and the function should be idempotent properly.
  - No diagnostic output when YAPL detects an existing prefix — the user gets no feedback about what was found or skipped.

* **Impact/Need:** Users bringing their existing Lutris setups to YAPL hit confusing failures. The `system` Wine path is advertised in docs but broken.

---

## 2. Proposed Solution

Fix each issue with targeted, well-tested changes. The prefix detection logic should be robust enough to handle the two common structures (flat prefix root, and Proton-style `pfx/` subdirectory). Add explicit error returns for `filepath.Abs` failures. Fix `system` Wine prefix creation. Make `restructureProtonPrefix` idempotent by using `os.Lstat` instead of `os.Stat`.

---

## 3. Scope & Design Details

### 3a. Robust Prefix Detection

* **Behavior/Rule:** YAPL must detect a prefix regardless of whether `system.reg` is at the prefix root or inside a `pfx/` subdirectory.
* **Specifics:** Replace the single `os.Stat(system.reg)` check with a helper `prefixIsInitialized(path string) bool` that checks for `system.reg` both at the root and in `pfx/`. If found in `pfx/` (Proton's raw layout), run `restructureProtonPrefix` to bring it to the standard layout before returning. Print a clear message so the user knows an existing prefix was detected.
* **File:** `internal/command/command.go`

### 3b. Fix `restructureProtonPrefix` Idempotency

* **Behavior/Rule:** Calling `restructureProtonPrefix` on an already-restructured prefix (where `pfx` is a symlink to `.`) must be a safe no-op.
* **Specifics:** Change `os.Stat(pfxDir)` to `os.Lstat(pfxDir)`. If the result is a symlink, return immediately — the prefix is already in standard layout.
* **File:** `internal/command/command.go` (`restructureProtonPrefix`)

### 3c. Fix Silent `filepath.Abs` Errors

* **Behavior/Rule:** Path resolution failures must be surfaced as errors, not silently ignored.
* **Specifics:** Replace all four `protonBasePath, _ := filepath.Abs(...)` calls with proper `protonBasePath, err := filepath.Abs(...)` followed by error handling. Return a descriptive error if the path cannot be resolved.
* **File:** `internal/command/command.go` (lines ~36, ~125, ~164, ~236)

### 3d. Fix `ProtonVersion: "system"` Prefix Init

* **Behavior/Rule:** When `proton_version` is set to `"system"`, YAPL should use the system `wine64` or `wine` binary to initialise the prefix.
* **Specifics:** Detect `ProtonVersion == "system"` early in `InitializePrefix`. In this case, find `wine64` (or `wine`) via `$PATH` using `exec.LookPath`. Create the prefix by running `wine64 winecfg` with `WINEPREFIX` set, then exit. Skip all proton-script and restructuring steps.
* **File:** `internal/command/command.go`

### 3e. Lutris Prefix Import Guidance (Doc + Diagnostic)

* **Behavior/Rule:** When YAPL detects a `pfx/` structure (Lutris layout), print a clear message telling the user what was found and that it was migrated to the standard layout.
* **Specifics:** After `restructureProtonPrefix` runs on a detected Lutris-layout prefix, print: `"-> Detected Lutris/Proton-style prefix layout (pfx/ subdirectory). Migrated to standard YAPL layout."`. Add a section to the developer guide explaining how to import from Lutris.
* **File:** `internal/command/command.go`, `docs/developer-guide.md`

---

## 4. Execution & Milestones

- [ ] Write failing test: `prefixIsInitialized` detects both flat and pfx/ layouts
- [ ] Write failing test: `restructureProtonPrefix` is a no-op when pfx is already a symlink
- [ ] Write failing test: `InitializePrefix` returns an error when `filepath.Abs` fails
- [ ] Write failing test: `ProtonVersion: "system"` uses `exec.LookPath` to find wine
- [ ] Fix `restructureProtonPrefix` to use `os.Lstat` → tests pass
- [ ] Add `prefixIsInitialized` helper; update `InitializePrefix` to use it → tests pass
- [ ] Fix all four silent `filepath.Abs` errors → tests pass
- [ ] Implement `system` Wine path in `InitializePrefix` → tests pass
- [ ] Add Lutris migration message + developer guide section
- [ ] `go test ./...` green
- [ ] Move PRD to `prds/done/`

---

## 5. Technical Notes

### Key Files & Systems Targeted

* `internal/command/command.go` — `InitializePrefix`, `restructureProtonPrefix`, all `filepath.Abs` call sites
* `docs/developer-guide.md` — Lutris import section

### Risks & Considerations

* **`prefixIsInitialized` false positives:** A malformed directory that happens to have a `system.reg` file will be treated as an initialised prefix. This is acceptable — the real prefix check is hard to make foolproof and this is consistent with what Wine itself does.
* **`system` Wine via `exec.LookPath`:** The PATH at YAPL run time must contain the system Wine binary. This is expected for users who specifically opt into `proton_version: "system"`. Document the requirement.
* **Lutris prefix structure variations:** Lutris has changed its prefix storage location multiple times. The two most common structures (`prefix/system.reg` and `prefix/pfx/system.reg`) cover 95%+ of cases. Edge cases (e.g., Lutris sandboxed runners) are out of scope.
