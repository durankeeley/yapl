# PRD-43 - Shared Component Layers and `yapl push`

**Status:** `[ ] Pending`
**Priority:** High
**Size:** L
**Sprint:** Phase 2 — Registry server
**Tags:** `server`, `registry`, `push`, `components`
**Created:** 2026-10-04
**GitHub Issue:** N/A

---

## 1. Problem & Context

* **Current State:** The only way to make a package installable offline is `--bundle-deps`, which copies Proton, the runtime and DXVK into **every** archive (several GB each). Getting a package onto the server means copying files into its folder by hand.
* **Impact/Need:** Docker solved this with shared layers and `docker push`. Ten games on GE-Proton11-7 should store and transfer that Proton build once.

## 2. Proposed Solution

* Components (Proton builds, runtimes, DXVK, VKD3D, umu-launcher) become first-class registry objects, stored once under `components/<kind>/<version>/`.
* `yapl push <name>` uploads the package and any components the server is missing.

## 3. Scope & Design Details

### Component storage & API
```
components/<kind>/<version>/component.tar.zst
components/<kind>/<version>/manifest.json   {kind, version, sha256, size, version_info}
```
* `version_info` is the `config.VersionInfo` (bin_path, library paths) so the client can register it in `runner.json` without the original URL.
| GET | `/v1/components` | list |
| GET/HEAD | `/v1/components/{kind}/{version}/manifest` | metadata / existence check |
| GET | `/v1/components/{kind}/{version}/archive` | bytes |
| PUT | `/v1/components/{kind}/{version}` | upload (multipart: manifest, archive) |
| PUT | `/v1/packages/{name}/{version}` | upload (multipart: manifest, cover?, archive) |

### Upload rules
* `serve --allow-push` enables PUT; disabled by default. With `--auth open` the server prints a warning that anyone on the LAN can push.
* The server streams the archive part to a temporary file in the target directory while hashing it. If the SHA-256 differs from the manifest it deletes the file and answers 400. Otherwise it renames the staging directory into place, so readers never see half a version.
* An existing version is immutable: 409 Conflict.

### `yapl push <name> [--server] [--version]`
* Runs `package` into a temp dir (or uses `--archive <path>` with its sidecar), checks `/v1/components/.../manifest` for each component, packs and uploads only the missing ones (tar.zst, streamed through `io.Pipe`, never fully in memory), then uploads the package.
* Progress uses `internal/progress`.

## 4. Execution & Milestones

- [ ] Component store + API + tests.
- [ ] Multipart upload handlers with verify-then-rename + tests (checksum mismatch, conflict, push disabled).
- [ ] Client `PushPackage` / `PushComponent` streaming + tests against `httptest`.
- [ ] `push` command, README "Publishing to the server" section.

### Acceptance scenarios
* **Given** a server that already has `proton/GE-Proton11-7` **when** I push a second game using it **then** only the game archive is uploaded.
* **Given** a manifest whose SHA-256 doesn't match the uploaded bytes **then** the server answers 400 and nothing appears in the catalog.
* **Given** `serve` without `--allow-push` **when** I push **then** I get a clear "push is disabled on this server" error.
* **Given** Doom 1.0.0 already on the server **when** I push Doom 1.0.0 again **then** I get "version already exists; bump --version".
