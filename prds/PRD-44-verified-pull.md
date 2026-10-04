# PRD-44 - Pull v2: Tags, Verified Streaming Install, Components, Remembered Server

**Status:** `[ ] Pending`
**Priority:** High
**Size:** L
**Sprint:** Phase 2 — Registry server
**Tags:** `client`, `pull`, `registry`
**Created:** 2026-10-04
**GitHub Issue:** N/A

---

## 1. Problem & Context

* **Current State:** `pull` lists the whole catalog to find one entry, downloads into a temp file (double disk usage), extracts without verifying anything, can't choose a version, ignores components, and re-discovers the server every time.
* **Impact/Need:** Pull is the core of the console's "install" button. It must be one action that leaves a ready-to-play game, or nothing at all.

## 2. Proposed Solution

```
yapl pull "Doom"           # latest
yapl pull "Doom:1.2.0"     # specific tag
yapl pull list
```
* Resolve the manifest via `/v1/packages/{name}/{tag}/manifest`.
* Stream archive → SHA-256 hasher → decompressor → tar extractor into a staging directory beside the target. Verify the hash at the end; on mismatch delete staging. Otherwise rename into `games/<Name>`.
* Fetch every missing component the same way into `proton/` or `dependencies/` and register it in `runner.json` (MergeGlobal, local entries win).
* Download the cover into the game directory; write `installed.json` (`{name, version, server, installed_at}`) used by upgrade.
* Remember the server: after a successful discovery or `--server`, save `client.json` (`server`, `auth`). Later commands use it without discovery. `--server` always overrides.
* If the server has no `/v1/info` (older YAPL), fall back to the legacy endpoints with a notice.

## 3. Scope & Design Details

### `internal/client`
* `Info()`, `Catalog()`, `Manifest(name, tag)`, `Install(ctx, name, tag, root, progress)`, `InstallComponent(...)`.
* `ParseReference("Doom:1.2.0") → (name, tag)`; default tag `latest`.
* `IsCompatible(serverInfo)` uses `version.IsCompatible` on API version.

### `internal/archive`
* `ExtractStream(r io.Reader, format, dest, strip)` exported; regular files are opened with `O_TRUNC` (current code leaves stale bytes when overwriting a longer file).

### Remembered settings (`internal/config`)
* `ClientSettings{Server, Auth}` in `client.json`; password is never written to disk.

## 4. Execution & Milestones

- [ ] Streaming verified install with tests: success, checksum mismatch leaves no directory, already-installed error.
- [ ] Component install + runner registration tests.
- [ ] `ParseReference` table tests; legacy fallback test.
- [ ] `client.json` persistence tests; README "Pulling games" rewritten.

### Acceptance scenarios
* **Given** Doom 1.2.0 on the server using GE-Proton11-7 **and** a client without that Proton **when** I `pull Doom` **then** `games/Doom`, `proton/GE-Proton11-7` and its `runner.json` entry all exist, and `installed.json` says 1.2.0.
* **Given** a corrupted archive on the server **when** I pull **then** the command fails with "checksum mismatch" and `games/Doom` does not exist.
* **Given** Doom already installed **when** I pull Doom **then** I'm told to use `yapl upgrade Doom`.
* **Given** a previous pull from 10.0.0.5:8471 **when** I `pull list` with no flags **then** no discovery broadcast is awaited.
