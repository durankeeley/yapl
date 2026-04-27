# PRD 2 - Remove win32 Architecture Code Path

**Status:** `[ ] Pending` | `[ ] In Progress` | `[x] Complete`
**Priority:** Medium
**Size:** M
**Sprint:** 2
**Tags:** `cleanup`, `wine11`, `win32`, `deprecated`
**Created:** 2026-04-27
**GitHub Issue:** N/A

---

## 1. Problem & Context

Wine 11.0 (January 2026) finalised its WoW64 mode: a 64-bit Wine process can now run 32-bit and 16-bit Windows applications without any 32-bit system libraries on the host. This makes all of YAPL's `win32` handling dead code.

* **Current State:** YAPL has a `wine_arch` config field, a `patchProtonForWin32` function that copies an entire Proton directory and does a naive string-replace of `wine64` → `wine`, and a separate `win32` branch in `InitializePrefix` that runs `winecfg` directly. Modern GE-Proton, CachyOS Proton, and other distributions are all built on Wine 9+ with WoW64 — `wine_arch: win32` is not needed.

* **Impact/Need:** Dead code increases maintenance burden, the string-replace patch is fragile (it can mangle library path names that contain "wine64"), and the separate win32 proton directory doubles disk usage unnecessarily. New contributors also get confused by this code path.

---

## 2. Proposed Solution

Remove all `win32` arch handling: the `patchProtonForWin32` function, the `win32` branch in `InitializePrefix`, the `win32` branches in `getWineExecutablePath` and `getProtonPath`, and the `WineArch` field from the `App` config struct. Update the developer guide and README to note that 32-bit apps run transparently on modern Proton.

---

## 3. Scope & Design Details

### 3a. Config: Remove `WineArch` Field

* **Behavior/Rule:** `wine_arch` in `game.json` and `app.json` is silently ignored (and later removed from the struct).
* **Specifics:** Remove `WineArch string` from `config.App`. Update `getWineArch` to always return `"win64"`. Remove the function entirely once callers are simplified.
* **File:** `internal/config/config.go` (App struct), `internal/command/command.go` (`getWineArch`)

### 3b. Dependency: Remove `patchProtonForWin32`

* **Behavior/Rule:** No patched Proton copy is ever created for win32.
* **Specifics:** Delete `patchProtonForWin32` in `dependency.go`. Remove the call site in `ensureProton`.
* **File:** `internal/dependency/dependency.go:143-172`

### 3c. Command: Remove win32 Branch in `InitializePrefix`

* **Behavior/Rule:** All prefix initialization uses the standard 64-bit Proton script path.
* **Specifics:** Delete the `if wineArch == "win32"` block (lines 32–66 in command.go). Simplify `InitializePrefix` to a single code path.
* **File:** `internal/command/command.go`

### 3d. Command: Simplify `getWineExecutablePath`

* **Behavior/Rule:** Only search for `wine64` then `wine` as fallback, no win32-specific logic.
* **Specifics:** Remove the `if wineArch == "win32"` branch.
* **File:** `internal/command/command.go` (`getWineExecutablePath`)

### 3e. Command: Simplify `getProtonPath`

* **Behavior/Rule:** No `-win32` suffix appended to proton version paths.
* **Specifics:** Remove the `if wineArch == "win32"` branch. The `wineArch` parameter can then be removed from the function signature entirely.
* **File:** `internal/command/command.go` (`getProtonPath`)

### 3f. Documentation Update

* **Specifics:** Add a note to README and developer-guide.md explaining that 32-bit apps run natively via WoW64 on Wine 11+ / modern Proton — no special config needed.

---

## 4. Execution & Milestones

- [ ] Write tests that confirm win32-specific code paths no longer exist (negative tests)
- [ ] Remove `WineArch` from `config.App`; simplify `getWineArch` to always return `"win64"` → tests pass
- [ ] Delete `patchProtonForWin32` and its call site → tests pass
- [ ] Remove win32 branch from `InitializePrefix` → tests pass
- [ ] Simplify `getWineExecutablePath` to remove win32 branch → tests pass
- [ ] Simplify `getProtonPath` to remove win32 suffix logic → tests pass
- [ ] Update README and developer guide
- [ ] `go test ./...` green
- [ ] Move PRD to `prds/done/`

---

## 5. Technical Notes

### Key Files & Systems Targeted

* `internal/config/config.go` — remove `WineArch` from `App` struct
* `internal/command/command.go` — simplify `InitializePrefix`, `getWineArch`, `getWineExecutablePath`, `getProtonPath`
* `internal/dependency/dependency.go` — delete `patchProtonForWin32`
* `docs/developer-guide.md`, `README.md` — add WoW64 note

### Risks & Considerations

* **Existing `game.json` files with `wine_arch: "win32"` set:** Go's JSON unmarshalling will silently ignore unknown fields, so old configs will continue to work (the field is simply ignored). No migration needed.
* **Anyone still on a Wine build older than 9:** If someone has a truly ancient Wine build without WoW64, they would need to use a system Wine setup manually. This is an acceptable edge case given the project's stated requirement of modern Proton builds.
