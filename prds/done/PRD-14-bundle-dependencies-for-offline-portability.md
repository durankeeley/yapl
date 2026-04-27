# PRD-14 - Bundle Proton/DXVK/Runtime into Package for True Offline Portability

**Status:** `[x] Complete`
**Priority:** High
**Size:** L
**Sprint:** Backlog
**Tags:** `feature`, `package`, `offline`, `portability`
**Created:** 2026-04-27

---

## 1. Problem & Context

YAPL's portability story currently assumes the target machine can reach the internet (or a local mirror) to download Proton, DXVK, VKD3D, and the Steam Linux Runtime on first run. At a LAN party, air-gapped lab, or offline deployment scenario, this is not possible. The game package is self-contained, but the runtime dependencies are not.

* **Current State:** `yapl package` archives only the game directory. Proton and dependencies must be downloaded separately on each target machine.
* **Impact/Need:** True "copy and run" portability is broken for offline environments. LAN operators must either pre-seed every machine with the right Proton version or rely on internet access.

## 2. Proposed Solution

* Add a `--bundle-deps` flag to the `package` command.
* When set, copy the game's configured Proton version, DXVK, VKD3D, and runtime directories into a `_bundle/` subfolder inside the archive alongside the game folder.
* On `unpackage`, detect the `_bundle/` folder and move its contents into the correct `proton/` and `dependencies/` directories on the target machine, skipping any that already exist.
* The resulting archive is fully self-contained: copy it, run `yapl unpackage`, and the game can launch without internet.

## 3. Scope & Design Details

### Bundle layout inside archive
* **Behavior/Rule:** The archive contains two top-level directories:
  ```
  mygame/           <- game dir (same as today)
  _bundle/
      proton/
          ge-proton-9/    <- full Proton build
      dependencies/
          dxvk/
              2.3/
          vkd3d/
              1.12/
          runtime/
              sniper/
  ```
* **Specifics:** `_bundle/` prefix is chosen to avoid collision with any game name. The underscore makes it visually distinct.

### Package changes
* **Behavior/Rule:** When `--bundle-deps` is passed, before creating the archive:
  1. Copy `proton/<version>/` to a temp staging dir as `_bundle/proton/<version>/`.
  2. Copy each configured dependency dir to `_bundle/dependencies/<name>/<version>/`.
  3. Include the temp staging dir alongside the game dir in the archive.
* **Specifics:** Use the existing `fs.CopyDir` function. Stage into `t.TempDir()` equivalent; clean up after archiving.

### Unpackage changes
* **Behavior/Rule:** After extraction, check whether a `_bundle/` directory exists at the archive root.
  - If yes: move each child of `_bundle/proton/` into `./proton/` (skip if already present), and each child of `_bundle/dependencies/` into `./dependencies/` (skip if already present). Delete `_bundle/` when done.
  - If no: existing behaviour unchanged.
* **Specifics:** "Skip if already present" means `fs.DirExistsAndIsNotEmpty` check before moving. Print a line for each dependency installed or skipped.

### Size warning
* **Behavior/Rule:** Before building a bundled package, estimate and print the total size:
  `Warning: bundled package will be approximately 8.3 GB. Continue? [y/N]:`
* **Specifics:** Sum directory sizes with `filepath.Walk`; divide by `1<<30` for GB. Prompt skipped with `--yes`.

### `runner.json` portability
* **Behavior/Rule:** Include the source machine's `runner.json` inside `_bundle/runner.json`.
* **Specifics:** On unpackage, if `runner.json` does not exist in the target directory, copy `_bundle/runner.json` there. If it exists, merge missing version entries (do not overwrite existing ones).

## 4. Execution & Milestones

- [ ] Write failing test: `TestPackage_BundleIncludesProtonDir`
- [ ] Write failing test: `TestPackage_BundleIncludesDependencies`
- [ ] Write failing test: `TestUnpackage_InstallsBundledProtonToProtonDir`
- [ ] Write failing test: `TestUnpackage_SkipsAlreadyPresentBundledDependency`
- [ ] Write failing test: `TestUnpackage_LeavesNoBundleDirAfterExtraction`
- [ ] Implement bundle staging in `app.Package`
- [ ] Add `--bundle-deps` and `--yes` flags to `package` in `main.go`
- [ ] Implement bundle detection + install in `app.Unpackage`
- [ ] Size estimation + confirmation prompt
- [ ] All tests pass
- [ ] Mark Complete, move to `prds/done/`

## 5. Technical Notes

### Key Files & Systems Targeted
* `internal/app/app.go` — `Package(opts PackageOptions)` and `Unpackage(targetDir string)`
* `internal/archive/archive.go` — may need multi-source archiving or accept a pre-staged directory
* `internal/fs/fs.go` — `CopyDir` already handles symlinks; reuse directly
* `cmd/yapl/main.go` — `--bundle-deps`, `--yes` flags on `package`

### Risks & Considerations
* **Archive size:** A fully bundled package (game + Proton + DXVK + runtime) can exceed 10 GB. Warn the operator and require confirmation.
* **Symlinks inside Proton:** GE-Proton contains many internal symlinks. `fs.CopyDir` already uses `os.Lstat` and `os.Symlink` to preserve these correctly.
* **Partial bundles:** If the user bundles Proton but has DXVK not yet downloaded, print a clear error rather than silently skipping. They must run `setup` first or the bundle will be incomplete.
* **`runner.json` merge:** A naive overwrite could break a target machine that has additional games configured. The merge strategy (add missing keys only) is safe.
* **Storage on target:** Unpackage must check available disk space before extracting a large bundle — out-of-space mid-extraction leaves a corrupt state.
