# PRD-41 - Package Manifest: Metadata, Versions, Checksums, Components

**Status:** `[ ] Pending`
**Priority:** High
**Size:** M
**Sprint:** Phase 2 — Registry server
**Tags:** `package`, `registry`, `metadata`
**Created:** 2026-10-04
**GitHub Issue:** N/A

---

## 1. Problem & Context

* **Current State:** A package is an anonymous tarball. The server knows only its file name and size. There is no title, cover art, version, checksum, or record of which Proton/runtime/DXVK it needs.
* **Impact/Need:** A store screen needs titles and cover art. Upgrades need versions. Safe LAN pulls need checksums. Shared layers (PRD-43) need the component list.

## 2. Proposed Solution

* Each game/app directory may contain `metadata.json` (authored by the operator). `yapl package` creates it with defaults if missing.
* `yapl package` writes two sidecar files next to the archive: `<Name>.manifest.json` (metadata + archive facts) and `<Name>.cover.<ext>` when the game has cover art.

## 3. Scope & Design Details

### `internal/manifest`
```go
type Metadata struct {
    Title, Version, Description string
    Tags []string
    Players string          // e.g. "1-16"
    Cover string            // file in the game dir, default cover.jpg/png/webp
    Preserve []string       // paths kept across upgrades (PRD-45)
}
type Component struct{ Kind, Version string }
type Manifest struct {
    SchemaVersion int        // const SchemaVersion = 1
    Name, Type string        // "game" | "app"
    Metadata
    Archive struct{ File, Format, SHA256 string; SizeBytes int64 }
    Components []Component   // derived from game.json
    Bundled bool             // archive carries _bundle/
    CreatedBy string         // "yapl <kernel version>"
}
```
* `LoadMetadata(dir)`, `DefaultMetadata(name)`, `ComponentsFor(config.App)`, `Write/Read(path)`, `Validate` (semver version, non-empty name).
* `ComponentsFor` lists proton (unless `system`), runtime (container method), dxvk, vkd3d, umu-launcher (umu, non-system).

### Packaging
* `yapl package <name> --version 1.2.0` writes the version into `metadata.json` before packaging.
* Archive is hashed while it is written (an `io.MultiWriter` into SHA-256), not re-read afterwards.
* `--output-dir <path>` puts archive and sidecars somewhere other than the current directory.
* Steam cover art: when `steam_app_id` is set and no cover exists, `--fetch-cover` downloads `library_600x900.jpg` from the Steam CDN. Off by default (no surprise network calls).

## 4. Execution & Milestones

- [ ] `manifest` package and tests (round trip, validation table, components table).
- [ ] Hash-while-writing in `archive`; returns archive facts.
- [ ] `package` writes sidecars; `--version`, `--output-dir`, `--fetch-cover`.
- [ ] README "Package metadata" section.

### Acceptance scenarios
* **Given** a game without `metadata.json` **when** I package it **then** `metadata.json` is created with title = name, version `1.0.0`, and `<Name>.manifest.json` lists the archive SHA-256 that matches `sha256sum <Name>.tar.xz`.
* **Given** a container-method game using GE-Proton11-7 and sniper **when** packaged **then** components are `proton/GE-Proton11-7` and `runtime/sniper`.
* **Given** `--version banana` **then** packaging fails before writing anything.
