# PRD-50 - Symlinked Games and Apps Are Part of the Library

**Status:** `[ ] In Progress`
**Priority:** High
**Size:** S
**Sprint:** Phase 3 follow-up
**Tags:** `bug`, `library`, `console`
**Created:** 2026-10-05
**GitHub Issue:** N/A

---

## 1. Problem & Context

* **Current State:** `games/aoe2hd -> ~/git/steam-rust/games/aoe2hd` (a valid game with `game.json`) is missing from `yapl list` and from the console. Directory scans use `os.DirEntry.IsDir()`, which is false for a symlink even when it points at a directory. `yapl run aoe2hd` works because lookup by name follows the link.
* **Impact/Need:** Keeping big games on another disk and symlinking them into the library is common. Every library view must treat a symlink to a directory as a directory.

## 2. Proposed Solution

* `fs.IsDir(path)` follows symlinks. Every library scan uses it: `app.ListAll`, the shared-Proton warning in `clean`, `console.ListLibrary` (and therefore `export`), `upgrade.Installed`, and the registry store's package scan.
* A dangling symlink is skipped, not an error.

## 4. Execution & Milestones

- [ ] Given/When/Then tests with real symlinks in each scanner.
- [ ] Scanners switched to `fs.IsDir`.
- [ ] README notes symlinked games are supported.
