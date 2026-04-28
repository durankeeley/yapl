# PRD-30 - DXVK/VKD3D auto-install for direct and local Wine builds

**Status:** `[ ] Pending`
**Priority:** High
**Size:** M
**Sprint:** —
**Tags:** `dxvk`, `vkd3d`, `direct`, `wine`
**Created:** 2026-04-28
**GitHub Issue:** —

---

## 1. Problem & Context

When `launch_method` is `container`, the Steam Linux Runtime + Proton script installs DXVK and VKD3D into the Wine prefix automatically — the user just sets `dxvk_version` in `game.json` and it works.

When `launch_method` is `direct` (standalone Wine build, e.g. wine-11.7), there is no Proton script to do this. DXVK and VKD3D DLLs must be manually copied into the prefix and registered with `WINEDLLOVERRIDES`. YAPL currently downloads the archives to `dependencies/dxvk/<version>/` and `dependencies/vkd3d/<version>/` but does nothing with them for `direct` mode — the game simply runs without them.

* **Current State:** DXVK/VKD3D are downloaded (if configured) but never installed into the prefix for `direct` method. Games that need DXVK for D3D9/D3D10/D3D11 or VKD3D for D3D12 will use Wine's native DX implementation, which is slower and less compatible.
* **Impact/Need:** Users on `direct` method can't get DXVK/VKD3D without manual prefix surgery.

---

## 2. Proposed Solution

During `setup` (and as a pre-check in `run`), detect when `launch_method` is `direct` or when proton_version uses a local/standalone Wine build (i.e. not an SLR Proton), and automatically install DXVK/VKD3D into the prefix using the DXVK/VKD3D install scripts or manual DLL copy.

The install mechanism mirrors what DXVK's own `setup_dxvk.sh` does:
1. Copy the appropriate DLLs (x64 for Wine 11 WoW64; x32+x64 for older Wine) from `dependencies/dxvk/<version>/` into `<prefix>/drive_c/windows/system32/` (and `syswow64/` for 32-bit).
2. Register them in the prefix registry with `WINEDLLOVERRIDES` set to `native` for each installed DLL.

DXVK ships a `setup_dxvk.sh` script that handles this. VKD3D-Proton ships a similar `setup_vkd3d_proton.sh`. YAPL should invoke these scripts (or replicate their DLL-copy logic if they're absent) when installing into a direct-method prefix.

---

## 3. Scope & Design Details

### Trigger condition

Each install step is opt-in and independent:

- **DXVK install** runs only if `dependencies.dxvk_version` is set in `game.json` AND `launch_method == "direct"`
- **VKD3D install** runs only if `dependencies.vkd3d_version` is set in `game.json` AND `launch_method == "direct"`
- Both also require the archive to be present at `dependencies/dxvk/<version>/` (i.e. `EnsureAll` ran first)

Container mode is excluded: the Proton script + SLR already handles DLL installation there. If neither version field is set, nothing happens — no auto-install, no error.

### Install mechanism

**DXVK:**
- Check if `dependencies/dxvk/<version>/setup_dxvk.sh` exists
  - If yes: run `WINEPREFIX=<absPrefix> bash setup_dxvk.sh install` with the appropriate wine env
  - If no: manually copy `.../x64/*.dll` into `<prefix>/drive_c/windows/system32/`
- Skip if DLLs are already present (idempotent check: `d3d11.dll` exists in system32)

**VKD3D-Proton:**
- Same pattern using `setup_vkd3d_proton.sh` or manual copy of `x64/*.dll`
- Skip if already installed

### Where in the codebase

- New function `dependency.InstallDXVKForDirect(prefixPath string, appCfg config.App, globalCfg config.Global, wineEnv []string) error`
- New function `dependency.InstallVKD3DForDirect(prefixPath string, appCfg config.App, globalCfg config.Global, wineEnv []string) error`
- Called from `App.Setup()` and `App.Run()` in `internal/app/app.go`, after `EnsureAll` and prefix init, only when `launch_method == "direct"`
- `command` package exposes a `BuildDirectWineEnv(prefixPath string, appCfg config.App, globalCfg config.Global, debug bool) ([]string, error)` helper so `dependency` can pass the right env to the install scripts without import cycles

### WINEDLLOVERRIDES

After installation, the `game.json` should have DXVK DLLs registered. Options:
1. **Auto-patch game.json**: add `d3d9`, `d3d10core`, `d3d11`, `dxgi` → `"native"` to `dll_overrides` on first install
2. **Rely on env var at launch**: set `WINEDLLOVERRIDES` in the launch env from configured `dll_overrides`

Prefer option 2 (env var at launch) — avoids mutating game.json and aligns with how container mode works. The DLLs copied into system32 are enough; Wine's override search order will pick up `native` copies when they exist in system32.

Actually, DXVK's `setup_dxvk.sh` also writes registry keys to force `native` load order. Replicating this without running the script requires a `reg add` command via wine. Simplest: invoke the DXVK install script if present; fall back to manual copy only.

---

## 4. Execution & Milestones

- [ ] Write failing tests:
  - `TestInstallDXVKForDirect_SkipsIfAlreadyInstalled` — if `d3d11.dll` exists in system32, function returns nil without copying
  - `TestInstallDXVKForDirect_CopiesDllsWhenNotPresent` — copies x64 DLLs into system32 when missing
  - `TestInstallDXVKForDirect_UsesSetupScriptWhenPresent` — if `setup_dxvk.sh` exists, it is invoked
- [ ] Run `go test ./...` — confirm new tests fail
- [ ] Implement `InstallDXVKForDirect` and `InstallVKD3DForDirect` in `internal/dependency/`
- [ ] Expose `BuildDirectWineEnv` helper in `internal/command/` (or move the env-building into a shared internal helper)
- [ ] Wire into `App.Setup()` and `App.Run()` in `internal/app/app.go`
- [ ] Run `go test ./...` — confirm all tests pass
- [ ] Update `README.md`: document that DXVK/VKD3D are auto-installed for `direct` method

---

## 5. Technical Notes

### Key Files & Systems Targeted

* `internal/dependency/dependency.go` — new `InstallDXVKForDirect`, `InstallVKD3DForDirect` functions
* `internal/command/command.go` — new `BuildDirectWineEnv` (or refactor existing env-building into a helper)
* `internal/app/app.go` — wire into `Setup()` and `Run()`
* `internal/dependency/dependency_test.go` — new test coverage

### DXVK archive layout (expected)

```
dependencies/dxvk/2.7.1/
  x32/   ← 32-bit DLLs (d3d9.dll, d3d10core.dll, d3d11.dll, dxgi.dll)
  x64/   ← 64-bit DLLs
  setup_dxvk.sh
```

### VKD3D-Proton archive layout (expected)

```
dependencies/vkd3d/2.14.1/
  x86/   ← 32-bit
  x86_64/ ← 64-bit
  setup_vkd3d_proton.sh
```

### Risks & Considerations

* **32-bit DLLs**: Wine 11 WoW64 handles 32-bit via the 64-bit process. `syswow64/` still needs the x32 DLLs if games load 32-bit d3d. Check Wine 11 WoW64 behaviour; may only need x64.
* **Import cycle**: `dependency` must not import `command`. `BuildDirectWineEnv` in `command` must be callable by the app layer, not by `dependency` directly. Wire env from `app.go`.
* **Idempotency**: Install must be safe to call on every `run`. Check DLL presence before copying.
* **setup_dxvk.sh may not exist**: Newer DXVK releases include it; check and fall back gracefully.
