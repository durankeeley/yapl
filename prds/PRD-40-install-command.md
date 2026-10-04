# PRD-40 - `yapl install`: Install a Game From an Installer or Folder

**Status:** `[ ] In Progress`
**Priority:** High
**Size:** M
**Sprint:** Phase 1 — Out-of-the-box foundation
**Tags:** `ux`, `install`, `cli`
**Created:** 2026-10-04
**GitHub Issue:** N/A

---

## 1. Problem & Context

* **Current State:** Adding a game takes five manual steps: `setup`, wait for Explorer, click through an installer, find the installed `.exe`, hand-edit `executable` in `game.json`.
* **Impact/Need:** Packaging is the operator's job on the "registry" machine. It should be one command, so building a LAN library is fast.

## 2. Proposed Solution

```
yapl install game "Doom" --installer ~/Downloads/setup_doom.exe
yapl install game "Half-Life" --from /mnt/cdrom/Half-Life
```

`install` runs setup (creating the config if missing), then either runs the installer inside the prefix or copies a portable game folder into `drive_c/Games/<name>`. Afterwards it detects the game's executable and writes it to the config.

## 3. Scope & Design Details

### Executable detection (`internal/detect`)
* `Snapshot(prefix)` lists every `.exe` under `drive_c`, excluding `drive_c/windows`.
* `Candidates(before, after, gameName)` returns new executables ranked: name similarity to the game name first, then file size. Executables matching the ignore list (`unins*`, `setup`, `install*`, `vcredist*`, `dxsetup`, `dotnet*`, `crash*`, `*report*`, `update*`) are excluded.
* Ranking weights are named constants.

### Command (`lifecycle` module)
* `--installer <file>`: run through the configured launch method with the installer as executable (absolute executable paths are now supported by the runners).
* `--from <dir>`: copy into `drive_c/Games/<name>` (no Wine needed).
* `--executable <relative path>`: skip detection.
* Detection only writes `executable` when the config still holds the placeholder (`drive_c/windows/explorer.exe`) or `--executable` is passed. It prints the ranked list so the user can change it.

## 4. Execution & Milestones

- [ ] `detect` package with table-driven tests.
- [ ] Runners accept absolute executable paths.
- [ ] `install` command with `--from` path tested end-to-end (no Wine).
- [ ] README "Adding a game" section and CLI help.

### Acceptance scenarios
* **Given** a portable game folder containing `hl.exe` and `uninstall.exe` **when** I run `yapl install game "Half-Life" --from <dir>` **then** the folder is in `drive_c/Games/Half-Life` and `executable` is `drive_c/Games/Half-Life/hl.exe`.
* **Given** an installer that creates `Doom.exe` and `unins000.exe` **when** detection runs **then** `Doom.exe` is ranked first and `unins000.exe` is excluded.
* **Given** a config whose `executable` was already customised **when** `install` finishes **then** the config is unchanged and the candidates are printed.

## 5. Technical Notes

### Key Files & Systems Targeted
* `internal/detect/*`, `internal/app/app.go` (`Install`), `internal/command/command.go` (absolute exe paths).

### Risks & Considerations
* **Installers that spawn and exit:** Some installers fork a child and return immediately. The container method's `waitforexitandrun` verb waits for the wineserver, which covers this; direct mode runs `wineserver --wait` after the installer.
