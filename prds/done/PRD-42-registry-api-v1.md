# PRD-42 - Registry API v1 and Versioned Storage

**Status:** `[x] Complete`
**Priority:** High
**Size:** L
**Sprint:** Phase 2 — Registry server
**Tags:** `server`, `registry`, `api`
**Created:** 2026-10-04
**GitHub Issue:** N/A

---

## 1. Problem & Context

* **Current State:** `serve` exposes `/packages` (name, type, format, size) and `/packages/<name>`. One file per name, no versions, no metadata, no cover art, no notion of server version. Package names come straight from the URL path.
* **Impact/Need:** The "Docker Hub" experience needs browsable metadata, tags (`doom:1.2.0`, `doom:latest`) and a stable, versioned API the console can rely on.

## 2. Proposed Solution

A storage layer (`internal/registry/store`) and an HTTP API (`internal/registry/api`), composed by the `serve` command. Storage and API are separate units with their own tests, so either can be replaced later (for example, by S3-backed storage).

### Storage layout (under `--packages-dir`)
```
games/<Name>/<version>/<Name>.tar.xz      versioned (written by push, PRD-43)
games/<Name>/<version>/manifest.json
games/<Name>/<version>/cover.jpg
games/<Name>.tar.xz                        legacy drop-in, still served
games/<Name>.manifest.json                 optional legacy sidecar (from `package`)
components/<kind>/<version>/...            PRD-43
```
* A legacy file without sidecar gets a synthesised manifest with version `0.0.0` and empty SHA-256 (clients warn but continue).
* Name and version segments are validated (`^[A-Za-z0-9][A-Za-z0-9 ._+-]*$`, no `..`), so the API can never be used for path traversal.

### API
| Method | Path | Purpose |
|---|---|---|
| GET | `/v1/info` | `{api_version, kernel, modules{name:version}, push_enabled}` |
| GET | `/v1/packages` | catalog: name, type, title, description, tags, latest, versions[], cover_url, size |
| GET | `/v1/packages/{name}` | all versions with full manifests |
| GET | `/v1/packages/{name}/{version}/manifest` | manifest (`latest` alias supported) |
| GET | `/v1/packages/{name}/{version}/archive` | archive bytes, `Range` supported |
| GET | `/v1/packages/{name}/{version}/cover` | cover image |
* "latest" = highest semantic version.
* Legacy `/packages` and `/packages/{name}` remain, backed by the same store.
* Basic-auth middleware unchanged; `/v1/info` is always public so clients can discover the auth mode.
* Every tunable (`ManifestRefreshInterval`, `BroadcastInterval`, port) comes from `defaults`.

## 4. Execution & Milestones

- [x] `store` package: scan, latest resolution, legacy compatibility, name validation — table-driven tests on `t.TempDir()` trees.
- [x] `api` package: handler tests via `httptest`.
- [x] `serve` command wired; `internal/server` kept as the composition root (store + API + rescan + broadcast) with its tests rewritten.
- [x] README "LAN package server" section rewritten with API table.

### Acceptance scenarios
* **Given** versions 1.0.0, 1.10.0, 1.2.0 of Doom **when** I GET `/v1/packages/Doom/latest/manifest` **then** I get 1.10.0.
* **Given** a legacy `games/Quake.tar.gz` **when** I GET `/v1/packages` **then** Quake is listed with version `0.0.0`.
* **Given** a request for `/v1/packages/..%2Fetc/latest/archive` **then** the server answers 400.
* **Given** password auth **when** I GET `/v1/info` without credentials **then** it succeeds; `/v1/packages` returns 401.
