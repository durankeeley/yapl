# PRD-35 - Display Resolution Management

**Status:** `[ ] Pending`
**Priority:** Medium
**Size:** L
**Sprint:** 3 — Validation & Deployment Flexibility
**Tags:** `feature`, `display`, `xrandr`, `kscreen-doctor`, `wayland`, `ux`
**Created:** 2026-05-16

---

## 1. Problem & Context

Older games (e.g. Counter-Strike 1.5, Half-Life, Battlefield 1942) were designed for fixed low resolutions and cannot change the host display themselves. On a modern 1440p/4K desktop with KDE Plasma Wayland, these games either render in a small window, stretch incorrectly, or fail to launch at the right resolution.

Many of these games also store their resolution in their own config files or registry keys (BF1942 stores `renderer.setResolution "1920x1080x32"` in `.con` files; older games use `Width=800` / `Height=600` across two lines). Currently the user must manually switch the host display resolution, update the game's own config, launch, and then reverse both changes on exit.

* **Current State:** YAPL launches the game without touching the host display or the game's config files. All resolution management is manual.
* **Impact/Need:** A LAN party with 10 machines means 10 people manually resetting display and game configs. This should be zero-touch.

## 2. Proposed Solution

Add an optional `display` block to `game.json`/`app.json`. When present, YAPL does the following around `yapl run`:

1. **Capture** — at the very start of `run`, record the current host display resolution. This is the reference point used for the post-exit check.
2. **Switch (opt-in)** — only if `set_host_resolution: true`, switch the host display to `resolution` before launching. Default is `false` — rely on the game to set its own display mode based on the config files YAPL patched.
3. **Patch the game's own config** — when `resolution` is set, find-and-replace the resolution value in the specified files and/or Wine registry keys so the game renders at the right resolution.
4. **Check and restore on exit** — after the game process exits, read the current host display resolution. If it differs from the value captured in step 1, restore it. If the game already reset the display correctly, YAPL does nothing.

The resolution is always specified directly in `game.json`. The `patch` entries are **write targets** — YAPL writes the configured resolution into those locations, not reads from them.

The post-exit check-and-restore (step 4) handles two cases: the game crashed without restoring its display change, or YAPL switched the display in step 2 and the game did not undo it. In both cases YAPL ensures the desktop returns to the state it was in before `yapl run` was called.

## 3. Scope & Design Details

### Config struct

Add to `internal/config/config.go`:

```go
type DisplayPatch struct {
    File     string `json:"file,omitempty"`     // path relative to prefix root
    Registry string `json:"registry,omitempty"` // Wine registry key path
}

type DisplayConfig struct {
    Output             string         `json:"output,omitempty"`
    Resolution         string         `json:"resolution,omitempty"`
    SetHostResolution  bool           `json:"set_host_resolution,omitempty"`
    Patch              []DisplayPatch `json:"patch,omitempty"`
}
```

`App` gains: `Display DisplayConfig` tagged `json:"display,omitempty"`.

**Fields:**

- `output` — display connector name. Omit to use the primary detected display.
  - X11/xrandr names: `HDMI-1`, `DP-1`, `eDP-1`
  - KDE kscreen-doctor names: numeric output ID (`1`, `2`) or connector name
- `resolution` — the target resolution. Required if `patch` is set or `set_host_resolution` is `true`. Formats accepted:
  - `"800x600"` — width × height
  - `"1920x1080x32"` — width × height × bit depth (BF1942 style)
  - `"1024x768x16"` — 16-bit variant
  - Bit depth is carried through to file/registry patching but stripped when switching the host display (`xrandr`/`kscreen-doctor` use W×H only).
- `set_host_resolution` — default `false`. When `true`, YAPL switches the host display to `resolution` before launching. When `false`, YAPL relies on the game to change the display itself (using the patched config files). In both cases YAPL checks the display after exit and restores if needed.
- `patch` — list of locations to find-and-replace the resolution before launch. Each entry is either a `file` (path relative to `games/<name>/prefix/`) or a `registry` key path. Both can appear in the same list. YAPL saves the original content before patching and writes it back after the game exits.

**Example `game.json` — BF1942, patch game config only (default, no host switch):**

```json
{
  "proton_version": "ge-proton-9",
  "launch_method": "direct",
  "executable": "drive_c/BF1942/BF1942.exe",
  "display": {
    "resolution": "1920x1080x32",
    "patch": [
      { "file": "drive_c/BF1942/mods/bf1942/Settings/VideoDefault.con" },
      { "file": "drive_c/BF1942/mods/bf1942/Settings/Video.con" }
    ]
  }
}
```

YAPL writes `1920x1080x32` into the `.con` files, then relies on BF1942 to set the display itself. After exit, if BF1942 left the display at the wrong resolution, YAPL restores it.

**Example `game.json` — older game that cannot change display itself:**

```json
{
  "proton_version": "ge-proton-9",
  "launch_method": "direct",
  "executable": "drive_c/HalfLife/hl.exe",
  "display": {
    "output": "HDMI-1",
    "resolution": "800x600",
    "set_host_resolution": true,
    "patch": [
      { "file": "drive_c/HalfLife/valve/opengl.cfg" }
    ]
  }
}
```

YAPL switches `HDMI-1` to `800x600`, writes the resolution into `opengl.cfg`, launches the game, and after exit checks if the display is still at `800x600` — if so, restores to the desktop resolution captured before launch.

---

### `internal/display` package

New package with three files:

```
internal/display/display.go    — public API + backend selection
internal/display/xrandr.go     — X11 backend
internal/display/kscreen.go    — KDE Wayland backend (kscreen-doctor)
```

#### Backend selection

At runtime, YAPL selects the backend in this order:

1. If `$WAYLAND_DISPLAY` is set or `$XDG_SESSION_TYPE == "wayland"`:
   - Look for `kscreen-doctor` in `PATH` → use KDE backend.
   - If not found, return an error: `display: KDE Wayland detected but kscreen-doctor not found in PATH`.
2. Otherwise (X11):
   - Look for `xrandr` in `PATH` → use X11 backend.
   - If not found, return an error: `display: X11 session detected but xrandr not found in PATH`.

#### Backend interface (internal)

```go
type backend interface {
    Primary() (output, resolution string, err error)
    ListModes(output string) ([]string, error)
    Apply(output, resolution string) error
    Restore(output, original string) error
}
```

#### Public API

```go
// Apply switches the named output (empty = primary) to the parsed resolution.
// Returns the original mode string for passing to Restore.
func Apply(output, resolution string) (original string, err error)

// Restore switches the named output back to the original mode string.
func Restore(output, original string) error

// Primary returns the name and current mode of the primary connected output.
func Primary() (output, resolution string, err error)
```

#### X11 backend (`xrandr.go`)

- `Primary()`: runs `xrandr --query`, finds the connected output with `*` on its current mode line.
- `Apply()`: runs `xrandr --output <output> --mode <WxH>` (bit depth stripped, not applicable to xrandr).
- `Restore()`: same call with the original mode string.
- Mode unavailable: `xrandr` exits non-zero → YAPL returns `display: mode 800x600 not available on output HDMI-1`.

#### KDE Wayland backend (`kscreen.go`)

- `Primary()`: runs `kscreen-doctor -o`, parses the output to find the enabled output marked as primary. Extracts its current mode (`Geometry` line or `Modes:` line with `*`).
- `Apply()`: runs `kscreen-doctor output.<ID>.mode.<WxH>`. If no exact match for `WxH`, tries `<WxH@60>` then the first mode matching the width. Reports available modes if none match.
- `Restore()`: runs `kscreen-doctor output.<ID>.mode.<original>`.
- Output ID: `kscreen-doctor -o` lists outputs with numeric IDs and connector names (e.g. `Output: 2 HDMI-A-1 enabled connected`). When `output` in the config is a connector name (e.g. `HDMI-1`), match it case-insensitively against both the ID and connector name fields.

---

### Resolution format parsing

The `resolution` string is parsed into a `Resolution` struct before any use:

```go
type Resolution struct {
    Width    int
    Height   int
    BitDepth int // 0 = not specified
}
```

Parse rules:
- `"800x600"` → `{800, 600, 0}`
- `"1920x1080x32"` → `{1920, 1080, 32}`
- `"1024x768x16"` → `{1024, 768, 16}`
- Separator is `x` (case-insensitive). Spaces around `x` are trimmed.
- If any component is non-numeric or zero, return a parse error.

`Resolution.String()` → `"WxH"` (no bit depth, used for host display commands).
`Resolution.GameString()` → `"WxH"` if BitDepth == 0, else `"WxHxBD"` (used for game config patching).

---

### File patching

For each `patch.file` entry, YAPL performs find-and-replace using the following pattern list in order. Each pattern that matches is replaced. Unmatched patterns are silently skipped (not an error — a file may only use one pattern style).

**Pattern 1 — inline `WxH` or `WxHxBD` (single line)**

Regex: `(\d+)x(\d+)(?:x(\d+))?`

Examples matched:
```
resolution=1920x1080
videoMode "1024x768x32"
renderer.setResolution "1920x1080x32"
```

Replacement: always write `Resolution.GameString()` — `"WxH"` if no bit depth was specified in `game.json`, `"WxHxBD"` if it was. The format of the value already in the game's config file is irrelevant; the replacement is always driven by what the user wrote in `game.json`.

**Pattern 2 — split key=value across lines**

Two sub-patterns applied independently:

Width line regex: `(?i)((?:screen)?width\s*[=\s]\s*)(\d+)`
Height line regex: `(?i)((?:screen)?height\s*[=\s]\s*)(\d+)`

Examples matched:
```
Width=800
Height=600

ScreenWidth = 1024
ScreenHeight = 768

renderer.setScreenWidth 1920
renderer.setScreenHeight 1080
```

Replacement: preserve the key and separator exactly, replace only the numeric value.

**Backup and restore:**
Before patching, YAPL saves the original file content in memory (not on disk). After the game exits, the original bytes are written back to the file. If the game crashes or YAPL is interrupted, the defer path runs the restore.

**File path resolution:**
Paths in `patch.file` are relative to `games/<name>/prefix/` (the Wine prefix root). YAPL returns an error if the file does not exist: `display: patch file not found: <path>`.

---

### Registry patching

For each `patch.registry` entry, YAPL queries and sets values using the Proton Wine binary for this game.

**Read (before launch, to save original values for restore):**
```
wine reg query "<key>" /v Width
wine reg query "<key>" /v Height
```
Parse `REG_DWORD` or `REG_SZ` values. If the key doesn't exist, YAPL skips registry patching for that entry (not an error — the game may not have been run yet to create the key).

**Write (resolution application):**
```
wine reg add "<key>" /v Width /t REG_DWORD /d <W> /f
wine reg add "<key>" /v Height /t REG_DWORD /d <H> /f
```
If `BitDepth > 0`, also write:
```
wine reg add "<key>" /v BitsPerPixel /t REG_DWORD /d <BD> /f
```

**Restore:** write back the saved original values using the same `reg add` commands with the original data. If the key didn't exist before, delete the values with `reg delete`.

---

### `app.Run` integration

In `internal/app/app.go`:

```
if cfg.Display is non-zero:
    1. resolve output (use cfg.Display.Output or detect Primary())
    2. capture originalMode = current resolution of that output
    3. if cfg.Display.SetHostResolution && cfg.Display.Resolution != "":
           display.Apply(output, resolution)   ← switch host BEFORE launch
    4. if cfg.Display.Resolution != "":
           for each patch entry:
               save original content in memory
               apply find-and-replace
    5. launch game (blocking cmd.Wait)
    6. restore patch files (write original bytes back)
    7. currentMode = current resolution of that output
       if currentMode != originalMode:
           display.Restore(output, originalMode)  ← only if display changed
```

Steps 6 and 7 run in a deferred cleanup function so they execute even if the game crashes or `cmd.Wait` returns an error. The display restore in step 7 is conditional — if the game correctly reset the display itself, YAPL does nothing. If the game changed the display and did not restore it (crash, forced close, or Wine bug), YAPL restores to the captured `originalMode`.

---

### Validation (add to PRD-8's pass)

- `display.resolution` is set but does not parse → error: `display.resolution "X" is not a valid resolution (expected WxH or WxHxBitDepth)`.
- `display.set_host_resolution: true` but `display.resolution` is empty → error: `set_host_resolution requires resolution to be set`.
- `patch` entries present but `display.resolution` is empty → error: `display.patch requires resolution to be set`.
- `patch` entry has both `file` and `registry` set → error: `display.patch entry must set either file or registry, not both`.
- `patch` entry has neither `file` nor `registry` set → error: `display.patch entry must set file or registry`.
- `display` block present with no `resolution`, no `patch`, and `set_host_resolution: false` → error: `display block has nothing to do; set resolution and/or patch`.

---

## 4. Execution & Milestones

- [ ] Add `DisplayConfig`, `DisplayPatch`, `Resolution` types to `internal/config/config.go`
- [ ] Add `Display DisplayConfig` field to `App` struct
- [ ] Write failing tests in `internal/display/display_test.go`:
  - `TestParseResolution_Formats` (table-driven: `800x600`, `1920x1080x32`, `1024x768x16`, bad inputs)
  - `TestXrandrBackend_Primary` (stub `xrandr` via PATH script in `t.TempDir()`)
  - `TestXrandrBackend_ApplyAndRestore`
  - `TestXrandrBackend_SetHostResolutionFalse_DoesNotSwitch`
  - `TestXrandrBackend_PostExit_RestoresOnlyIfDisplayChanged`
  - `TestXrandrBackend_ModeNotAvailable_ReturnsError`
  - `TestKscreenBackend_Primary` (stub `kscreen-doctor` via PATH script)
  - `TestKscreenBackend_ApplyAndRestore`
  - `TestKscreenBackend_ConnectorNameMatch`
  - `TestBackendSelection_WaylandPicksKscreen` (set `WAYLAND_DISPLAY` via `t.Setenv`)
  - `TestBackendSelection_X11PicksXrandr`
- [ ] Write failing tests in `internal/display/patch_test.go`:
  - `TestPatchFile_InlineFormat` (single-line `WxH` and `WxHxBD`)
  - `TestPatchFile_SplitKeyValue` (`Width=` / `Height=` format)
  - `TestPatchFile_BF1942Style` (`renderer.setScreenWidth` format)
  - `TestPatchFile_MultiplePatterns_AllReplaced`
  - `TestPatchFile_NoMatch_NoError`
  - `TestPatchFile_Restore_WritesOriginalBack`
  - `TestPatchRegistry_WritesAndRestores` (stub `wine` binary via PATH)
- [ ] Implement `internal/display/display.go`, `xrandr.go`, `kscreen.go`, `patch.go`
- [ ] Integrate into `app.Run` with deferred restore for both host display and game config patches
- [ ] Add display validation to config validation pass
- [ ] Update `README.md` with `display` block examples (BF1942, Half-Life split-format)
- [ ] All tests pass (`go test ./...`)
- [ ] Mark Complete, move to `prds/done/`

## 5. Technical Notes

### Key Files & Systems Targeted

* `internal/config/config.go` — `DisplayConfig`, `DisplayPatch`, `Resolution` structs; `Display` field on `App`
* `internal/display/display.go` — public API, backend selection
* `internal/display/xrandr.go` — X11 backend
* `internal/display/kscreen.go` — KDE Wayland backend (`kscreen-doctor`)
* `internal/display/patch.go` — file and registry find-and-replace
* `internal/display/display_test.go` — backend and resolution tests
* `internal/display/patch_test.go` — patching tests
* `internal/app/app.go` — `Run` integration with deferred cleanup

### Risks & Considerations

* **kscreen-doctor output format:** KDE version updates can change `kscreen-doctor -o` output. The parser should be tolerant: look for `Output: <id> <connector>` lines and `Modes:` lines rather than relying on exact column positions. Add a test that covers a real `kscreen-doctor -o` fixture from a KDE Plasma 6 machine.
* **kscreen-doctor output IDs vs connector names:** The config may say `"HDMI-1"` but kscreen-doctor uses `"HDMI-A-1"` or a numeric ID. Normalise connector names for comparison (strip `-A-`, treat `-` and `_` as equivalent, compare case-insensitively).
* **Mode not in list:** `kscreen-doctor` will silently fail or error if the mode isn't in the output's supported mode list. `listModes()` should be called first, and if the target resolution isn't available, return a clear error listing what modes are available.
* **Signal handling:** `defer` does not run on unhandled OS signals (SIGKILL). For SIGTERM/SIGINT, the process exits normally and defers run. This is sufficient for the LAN party use case — SIGKILL is rare and requires operator intervention anyway.
* **BF1942 `.con` files:** These are text files with Battlefield's custom scripting syntax. Pattern 1 (inline `WxHxBD`) covers `renderer.setResolution "1920x1080x32"`. Pattern 2 (split `renderer.setScreenWidth`/`renderer.setScreenHeight`) covers games using that style. Both patterns run on the same file, so a file using both styles gets both updated.
* **Bit depth on host display:** Neither `xrandr` nor `kscreen-doctor` take bit depth as an input for mode switching — they operate on W×H (and optionally refresh rate). Bit depth is only used when writing to game config files/registry. This is intentional: modern Linux display servers don't support 16-bit modes.
* **Wine binary path:** The `wine` used for registry operations must be the same Proton Wine binary that owns the prefix. `app.Run` passes the resolved `wineExe` path after `dependency.EnsureAll` has run; `patch.go` accepts it as a parameter.
* **Non-existent patch file:** Return an error before launching rather than silently skipping, so the user knows their config path is wrong before waiting for the game to open.
