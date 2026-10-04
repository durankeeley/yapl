# PRD-48 - Desktop, Steam and Autostart Integration (`yapl export`)

**Status:** `[x] Complete`
**Priority:** Medium
**Size:** M
**Sprint:** Phase 3 — Console client
**Tags:** `console`, `integration`, `steam`, `desktop`
**Created:** 2026-10-04
**GitHub Issue:** N/A

---

## 1. Problem & Context

* **Current State:** Games are only reachable through the terminal. Many LAN machines (and every Steam Deck) already have a couch UI: Steam Big Picture. Desktop users expect app-menu entries.
* **Impact/Need:** Meet players where they are and allow a machine to boot straight into the console.

## 2. Proposed Solution

```
yapl export desktop [name]     # .desktop entries (all games, or one) + a "YAPL Console" entry
yapl export steam [name]       # add non-Steam shortcuts to Steam (Steam must be closed)
yapl export autostart          # start `yapl console` on login
```
* All entries call `yapl --workdir <abs root> run "<name>"` so they work from anywhere (PRD-26).

## 3. Scope & Design Details

### Desktop (`internal/integration/desktop`)
* Writes `$XDG_DATA_HOME/applications/yapl-<slug>.desktop` (default `~/.local/share/applications`). Icon = game cover if present. `--output-dir` overrides for packaging/testing.

### Steam (`internal/integration/steam`)
* Binary VDF reader/writer for `userdata/<id>/config/shortcuts.vdf` (types: map 0x00, string 0x01, int32 0x02, float32 0x03, uint64 0x07, end 0x08). Numeric values are kept as raw bytes so files round-trip byte-exactly.
* Every Steam user under `userdata/` gets the shortcuts (`--steam-user <id>` limits it to one).
* Lives in its own versioned module, `internal/modules/integration`.
* Merge: existing shortcuts are preserved; a YAPL shortcut is matched by its `Exe`+`LaunchOptions` and updated in place, not duplicated.
* Shortcut app ID = `crc32(exe + name) | 0x80000000`; cover copied to `config/grid/<appid>p.<ext>` so Big Picture shows art.
* Refuses when Steam is running (it rewrites the file on exit), unless `--force`.
* `--steam-root` override for tests; default `~/.local/share/Steam` then `~/.steam/steam`.

### Autostart
* Writes `$XDG_CONFIG_HOME/autostart/yapl-console.desktop` running `yapl --workdir <root> console`.

## 4. Execution & Milestones

- [x] VDF round-trip tests (byte-exact on a fixture), merge tests.
- [x] Desktop file content tests (quoting of names with spaces).
- [x] App ID test against the documented formula (crc32 of Exe+AppName with the high bit set).
- [x] README "Integrations" section.

### Acceptance scenarios
* **Given** an existing `shortcuts.vdf` with one user shortcut **when** I export two games **then** the file has three shortcuts and the user's is byte-identical.
* **Given** I export twice **then** there are still exactly two YAPL shortcuts.
* **Given** "Need for Speed: Most Wanted" **then** the desktop file is `yapl-need-for-speed-most-wanted.desktop` with a correctly quoted Exec line.
