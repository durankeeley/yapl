# PRD-37 - Merge Bundled `runner.json` Entries on Unpackage

**Status:** `[x] Complete`
**Priority:** High
**Size:** S
**Sprint:** Phase 0 — Stabilise
**Tags:** `bug`, `unpackage`, `pull`, `config`
**Created:** 2026-10-04
**GitHub Issue:** N/A

---

## 1. Problem & Context

* **Current State:** `installBundledDeps` copies the bundled `runner.json` only when the target machine has no `runner.json`. After the first pull, every later bundled package's version entries are thrown away.
* **Impact/Need:** The second game pulled onto a machine fails with `proton version '<x>' not defined in runner.json` even though the Proton build itself was installed from the bundle. This breaks the "pull many games onto one console" flow.

## 2. Proposed Solution

* Merge the bundled `runner.json` into the local one: add every Proton, runtime and dependency version the local file does not define. Never overwrite an entry the local machine already defines.

## 3. Scope & Design Details

### Config (`internal/config`)
* **`LoadGlobal(path)` / `SaveGlobal(path, g)`:** read/write without the create-default side effect.
* **`MergeGlobal(local, bundled Global) (Global, []string)`:** returns the merged config and a sorted list of added keys (`proton/GE-Proton9-20`, `dependencies/dxvk/2.4`, …) for user-facing output.

### Archive (`internal/archive`)
* **Behavior/Rule:** `installBundledDeps` writes the bundled file when none exists, otherwise merges and prints each added entry.

## 4. Execution & Milestones

- [x] Gherkin-style tests for `MergeGlobal` (empty local, disjoint, overlapping keys keep local values).
- [x] Unpackage test: second bundle adds its Proton entry to an existing `runner.json`.
- [x] Implementation; `go test ./...` passes.
- [x] README updated (offline bundle section).

## 5. Technical Notes

### Key Files & Systems Targeted
* `internal/config/config.go` — load/save/merge helpers.
* `internal/archive/archive.go` — merge in `installBundledDeps`.

### Risks & Considerations
* **Local wins:** A conflicting version key with a different URL keeps the local value. That is deliberate: the operator's file is the source of truth.
