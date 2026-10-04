# PRD-39 - Working Defaults Out of the Box

**Status:** `[x] Complete`
**Priority:** High
**Size:** M
**Sprint:** Phase 1 — Out-of-the-box foundation
**Tags:** `ux`, `config`, `defaults`
**Created:** 2026-10-04
**GitHub Issue:** N/A

---

## 1. Problem & Context

* **Current State:** A fresh `runner.json` contains `EDIT_ME` placeholders, so nothing works until the user finds Proton and DXVK URLs and writes JSON. `LoadOrCreateApp` picks the "first" Proton key from a Go map, which is random order.
* **Impact/Need:** The very first command a new user runs ends in a confusing failure. A console-style product must work on first launch.

## 2. Proposed Solution

* Ship a versioned **defaults catalog**: a pinned, known-good set of components (GE-Proton11-7 x86_64, DXVK 3.1.1, VKD3D-Proton 3.0.1, Steam Linux Runtime sniper) compiled into the binary.
* Add a `defaults` block to `runner.json` naming the preferred Proton, runtime, DXVK and VKD3D versions. New game configs use it, deterministically.

## 3. Scope & Design Details

### `internal/catalog`
* `const Version = "2026.10.0"` (calendar versioned; bumped whenever pins change).
* `Default() config.Global` returns the pinned components and the `defaults` block.

### `config.Global`
* New `Defaults struct{Proton, Runtime, DXVK, VKD3D string}` field (`json:"defaults,omitempty"`).
* `LoadOrCreateGlobal(path, fallback)` writes the fallback (callers pass `catalog.Default()`) instead of placeholders. The parameter avoids a `config` → `catalog` → `config` import cycle.
* `LoadOrCreateApp` chooses `Defaults.Proton`, then the sorted first key, never map order. `Defaults.Runtime` likewise. `defaults.dxvk`/`defaults.vkd3d` are recorded for later use. New configs don't set `dxvk_version` yet, because the default container method gets DXVK from Proton itself and direct-mode installation is PRD-30.

### Existing users
* An existing `runner.json` is never rewritten. Adding newer catalog pins to an existing file is handled by `yapl upgrade catalog` (PRD-45), which reuses `MergeGlobal` from PRD-37.

## 4. Execution & Milestones

- [x] Catalog package and tests.
- [x] `Defaults` field, deterministic selection, tests.
- [x] README Quick Start rewritten: setup works without editing JSON.

### Acceptance scenarios
* **Given** an empty directory **when** I run `yapl setup game "Doom"` **then** `runner.json` defines GE-Proton11-7, the sniper runtime, DXVK and VKD3D, and `games/Doom/game.json` uses GE-Proton11-7 with the container method.
* **Given** a `runner.json` with three Proton versions and `defaults.proton` set **when** a new game is created **then** it uses `defaults.proton`.
* **Given** no `defaults` block **when** a new game is created **then** it uses the alphabetically first Proton key.

## 5. Technical Notes

### Key Files & Systems Targeted
* `internal/catalog/catalog.go`, `internal/config/config.go`.

### Risks & Considerations
* **Pinned URLs age:** The catalog version makes this visible (`yapl version` shows it); updating pins is a one-file change.
