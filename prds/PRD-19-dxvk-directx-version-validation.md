# PRD-19 - Validate DXVKDirectXVersion Config Field

**Status:** `[ ] Pending`
**Priority:** Medium
**Size:** S
**Sprint:** Backlog
**Tags:** `bugfix`, `config`, `validation`, `dxvk`
**Created:** 2026-04-27

---

## 1. Problem & Context

`dependency.InstallCustomComponents` maps `deps.DXVKDirectXVersion` to a list of DLLs via:

```go
dxvkMap := map[string][]string{
    "9":  {"d3d9.dll"},
    "10": {"d3d10.dll", "d3d10_1.dll", "d3d10core.dll", "d3d11.dll", "dxgi.dll"},
    "11": {"d3d11.dll", "dxgi.dll"},
}
```

If a user sets `dxvk_directx_version` to any value other than `"9"`, `"10"`, or `"11"` (e.g., `"12"`, `"dx11"`, `""`, or a typo), the map lookup returns a nil slice, `install` is called with `len(dlls) == 0`, and the function silently does nothing — no DLLs are installed and no error is returned.

* **Current State:** Invalid `dxvk_directx_version` values are silently ignored; the user receives no feedback that their config is wrong.
* **Impact/Need:** DXVK DLLs are never installed, the game crashes, and the user has no idea why.

## 2. Proposed Solution

* Add validation of `DXVKDirectXVersion` in `config.ValidateApp` (PRD-8) — it must be one of `"9"`, `"10"`, `"11"`, or empty.
* Additionally, add a guard in `InstallCustomComponents` that returns an error when a non-empty `DXVKDirectXVersion` maps to a nil/empty DLL list.
* This is defence-in-depth: validation at config load and a safety net at install time.

## 3. Scope & Design Details

### Config validation (PRD-8 integration)
* **Behavior/Rule:** If `DXVKVersion` is non-empty and `DXVKDirectXVersion` is also non-empty, it must be one of `"9"`, `"10"`, `"11"`.
* **Specifics:** Add to `ValidateApp` in `config.go`. Error message: `dxvk_directx_version must be "9", "10", or "11" (got "12")`.

### InstallCustomComponents guard
* **Behavior/Rule:** After the map lookup, if `dlls` is empty and `installPath != ""` and `version != ""`, return an error: `unrecognised dxvk_directx_version "X"; valid values are 9, 10, 11`.
* **Specifics:** Move the nil guard from inside `install()` to before the call, so we can return a meaningful error.

## 4. Execution & Milestones

- [ ] Write failing test: `TestInstallCustomComponents_ErrorsOnUnknownDXVKDirectXVersion`
- [ ] Write failing test: `TestInstallCustomComponents_NoopWhenVersionEmpty`
- [ ] Add validation to `InstallCustomComponents` for unknown DirectX version
- [ ] Add to `config.ValidateApp` (coordinate with PRD-8)
- [ ] All tests pass
- [ ] Mark Complete, move to `prds/done/`

## 5. Technical Notes

### Key Files & Systems Targeted
* `internal/dependency/dependency.go` — `InstallCustomComponents` guard
* `internal/config/config.go` — `ValidateApp` addition (coordinate with PRD-8)
* `internal/dependency/dependency_test.go` — new tests

### Risks & Considerations
* **PRD-8 dependency:** If PRD-8 is implemented first, the validation goes there. If this PRD is implemented first, add the validation in `InstallCustomComponents` and leave a TODO for PRD-8 to move it to config validation.
* **Empty version is valid:** `DXVKDirectXVersion: ""` means no custom DLL installation — this must remain a no-op.
