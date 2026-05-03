# PRD-25 - Integrate Wine Builder Output with YAPL runner.json

**Status:** `[ ] Pending`
**Priority:** Medium
**Size:** S
**Sprint:** 5 — Wine Builder Ecosystem
**Tags:** `wine-builder`, `config`, `integration`, `feature`
**Created:** 2026-04-27

---

## 1. Problem & Context

After `wine-builder` produces a `wine-<version>.tar.xz` tarball (PRD-23), there is no automated way to register the built Wine version in YAPL's `runner.json`. The user must manually edit `runner.json` to add a `path` entry pointing at the output directory, then configure their `game.json` to reference it. This is undocumented and error-prone.

Additionally, YAPL's `ensureProton` currently only supports two lookup modes: URL (download from internet) and `path` (use a pre-existing directory). It has no mode for a local tarball that needs to be extracted — meaning a built `wine-11.7.tar.xz` cannot be used directly without first extracting it manually.

* **Current State:** Wine builder output must be manually registered in `runner.json` and manually extracted before use.
* **Impact/Need:** The wine builder is disconnected from YAPL's dependency management, making locally-built Wine awkward to use.

## 2. Proposed Solution

* Add a `local_archive` field to `VersionInfo` in `config.go`:
  ```json
  "proton_versions": {
    "wine-11.7": { "local_archive": "/path/to/wine-builder/output/wine-11.7.tar.xz" }
  }
  ```
* When `ensureProton` encounters a `local_archive` entry, extract the tarball to `proton/<version>/` using the existing `archive.Extract` logic, then mark it as done. Subsequent runs skip re-extraction (directory non-empty check).
* Add a `yapl add proton <name> <archive-path>` command that writes the `runner.json` entry automatically after a wine builder run.

## 3. Scope & Design Details

### VersionInfo.LocalArchive field
* **Behavior/Rule:** Add `LocalArchive string \`json:"local_archive,omitempty"\`` to `config.VersionInfo`.
* **Specifics:** Mutually exclusive with `URL` and `Path`. Validation: if more than one of `URL`, `Path`, `LocalArchive` is set, return a config error.

### ensureProton local_archive path
* **Behavior/Rule:** If `vinfo.LocalArchive != ""`:
  1. Check `os.Stat(vinfo.LocalArchive)` — error if file does not exist.
  2. Extract to `proton/<version>/` using `archive.Extract(protonPath, true)` with the local file as source.
  3. Cache: if `proton/<version>/` is already non-empty, skip.
* **Specifics:** Reuse `archive.Extract` — it already supports local file paths as source when `Source` does not start with `http`.

### `yapl add proton` command
* **Behavior/Rule:** `yapl add proton <name> <archive-path>` adds a `local_archive` entry to `runner.json`.
  ```bash
  yapl add proton wine-11.7 ./wine-builder/output/wine-11.7.tar.xz
  ```
  Follows the established pattern of `yapl <verb> <resource-type> <name>` (same as `yapl setup game <name>`, `yapl unpackage game <file>`).
* **Specifics:** Read `runner.json`, add the `proton_versions["wine-11.7"]` entry with `local_archive` set, write back. Error if the name already exists (user must use `--overwrite` to replace).

## 4. Execution & Milestones

- [ ] Write failing test: `TestEnsureProton_ExtractsLocalArchiveWhenSet`
- [ ] Write failing test: `TestEnsureProton_SkipsExtractionIfAlreadyPresent`
- [ ] Add `LocalArchive` to `config.VersionInfo`
- [ ] Add local_archive handling in `ensureProton`
- [ ] Implement `yapl add proton <name> <archive-path>` in `cmd/yapl/main.go` and `internal/app/app.go`
- [ ] All tests pass
- [ ] Mark Complete, move to `prds/done/`

## 5. Technical Notes

### Key Files & Systems Targeted
* `internal/config/config.go` — `LocalArchive` field in `VersionInfo`
* `internal/dependency/dependency.go` — `ensureProton` local archive path
* `internal/app/app.go` — `Add(name, archivePath string) error`
* `cmd/yapl/main.go` — `case "add":` dispatching on resource type `"proton"`

### Risks & Considerations
* **`archive.Extract` with local source:** Confirm that `archive.Extract` already handles `file://` paths or bare filesystem paths. If it only handles HTTP, add a local-file branch.
* **Mutual exclusivity:** A `VersionInfo` with both `URL` and `LocalArchive` set is a config error; validate in `config.ValidateApp` (PRD-8).
