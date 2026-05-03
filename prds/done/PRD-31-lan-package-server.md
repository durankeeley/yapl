# PRD-31 — LAN Package Server

**Status:** `[x] Complete`
**Priority:** High
**Size:** L
**Sprint:** —
**Tags:** `server`, `networking`, `lan`, `package-distribution`
**Created:** 2026-05-03
**GitHub Issue:** —

---

## 1. Problem & Context

* **Current State:** Distributing packaged games across LAN party machines requires manually copying archive files (e.g. via USB drive, `scp`, or shared filesystem). The `yapl unpackage` command must be run on each machine after copying. There is no built-in mechanism for a central host to serve packages to client machines over the network.
* **Impact/Need:** At a LAN party with multiple machines, one host can have the packaged games and clients should be able to request and receive them without manual file transfer. This keeps client machines lean — they need only `yapl` and network access to the server. The server must handle multiple simultaneous client requests (different clients, different games at the same time).

---

## 2. Proposed Solution

Add two new commands to YAPL:

1. **`yapl serve`** — starts an HTTP server that scans a configured packages directory, caches a manifest of available archives, and serves package listings and file downloads to clients.
2. **`yapl pull`** — a client command that connects to a `yapl serve` instance, lists available packages or downloads and unpacks a specific one.

The server uses standard `net/http` (stdlib). Auth is selectable per deployment: open, password (HTTP Basic Auth), or token (pre-shared bearer token). SSH key auth is a stretch goal requiring `golang.org/x/crypto/ssh` — see risks section.

---

## 3. Scope & Design Details

### Server Command (`yapl serve`)

* **Flags:**
  - `--packages-dir <path>` — directory to scan for `*.tar.gz`, `*.tar.xz`, `*.tar.zst` archives. Required.
  - `--port <int>` — TCP port to listen on. Default: `8471`.
  - `--auth <mode>` — auth mode: `open`, `password`, or `token`. Default: `open`.
  - `--password <string>` — required when `--auth password`. Shared password; Basic Auth username is ignored.
  - `--token <string>` — required when `--auth token`. Pre-shared bearer token clients must send as `Authorization: Bearer <token>`.
* **Package manifest:**
  - On startup: scan `--packages-dir` and build an in-memory `map[string]PackageEntry` where key is the bare archive filename (without extension), value holds full path, format, and byte size.
  - A background goroutine re-scans every 30 seconds so newly added archives become visible without restarting the server.
  - Only top-level files in the directory are listed (no recursive scan).
* **HTTP API:**
  - `GET /packages` → JSON array of `{ "name": "...", "format": "xz|gz|zst", "size_bytes": N }`. Size allows clients to show download progress.
  - `GET /packages/{name}` → streams the archive file as `application/octet-stream`. Name matches the bare filename key (no extension). Returns 404 if not found.
* **Concurrency:** `net/http` handles multiple simultaneous connections natively. File streaming uses `http.ServeContent` so range requests and concurrent reads work without custom locking.
* **Auth middleware:** a single wrapper around the `ServeMux` handler checks the auth mode and rejects with `401` before any handler runs. Applied to all routes.

### Client Command (`yapl pull`)

* **Subcommands / flags:**
  - `yapl pull --server <host:port> list` — print available packages (name, format, size).
  - `yapl pull --server <host:port> <name>` — download the named package into the current directory (or `--output-dir`) then call the existing `Unpackage` logic to extract into `games/` or `apps/` (determined by the archive's internal layout, same as the existing `unpackage` command).
  - `--auth <mode>` — must match server auth mode.
  - `--password <string>` — password when auth is `password`.
  - `--token <string>` — bearer token when auth is `token`.
  - `--output-dir <path>` — where to save the downloaded archive before unpackaging. Default: system temp dir; archive is removed after successful unpackage.
* **Download progress:** read `size_bytes` from the manifest then wrap the response body in a progress-counting reader that prints `Downloading <name>: X% (Y MB / Z MB)` to stderr.
* **Error behaviour:** if download fails mid-stream, delete the partial file and return an error. Never call `Unpackage` on a partial file.

### New Internal Package: `internal/server`

* `server.go` — `Server` struct, `NewServer(cfg Config) *Server`, `Start() error`, manifest cache, background scanner, HTTP handlers.
* `server_test.go` — table-driven tests: manifest scan, auth rejection (each mode), concurrent download simulation, 404 for unknown package, manifest refresh after adding a file.

### New Internal Package: `internal/client`

* `client.go` — `Client` struct, `NewClient(cfg Config) *Client`, `List() ([]PackageEntry, error)`, `Download(name, outputDir string) (string, error)` (returns path to downloaded archive).
* `client_test.go` — tests against an in-process `httptest.Server`: list round-trip, download round-trip (verify file bytes match), auth failure, server 404 propagation.

### Wire-up in `cmd/yapl/main.go`

* Add `serve` and `pull` to the command dispatch switch.
* `serve` builds a `server.Config` from flags and calls `server.NewServer(...).Start()`.
* `pull list` calls `client.List()` and prints.
* `pull <name>` calls `client.Download()` then the existing `archive.Unpackage()`.

---

## 4. Execution & Milestones

- [ ] Create `internal/server/server.go` and `server_test.go`; write failing tests first.
- [ ] Implement manifest scanning, background refresh, HTTP handlers, auth middleware.
- [ ] Run `go test ./internal/server/...` — confirm passing.
- [ ] Create `internal/client/client.go` and `client_test.go`; write failing tests first.
- [ ] Implement `List`, `Download`, progress display.
- [ ] Run `go test ./internal/client/...` — confirm passing.
- [ ] Wire `serve` and `pull` into `cmd/yapl/main.go`.
- [ ] Integration smoke test: `yapl serve` in one terminal, `yapl pull ... list` and `yapl pull ... <name>` in another.
- [ ] Update `README.md` with new commands and flags.
- [ ] Mark PRD Complete and move to `prds/done/`.

---

## 5. Technical Notes

### Key Files & Systems Targeted

* `cmd/yapl/main.go` — add `serve` and `pull` command dispatch and flag parsing.
* `internal/server/server.go` — new package; `Server`, `Config`, `PackageEntry` types; manifest cache; HTTP handlers.
* `internal/server/server_test.go` — new file; table-driven tests using `httptest`.
* `internal/client/client.go` — new package; `Client`, `Config` types; `List`, `Download`.
* `internal/client/client_test.go` — new file; tests against in-process `httptest.Server`.
* `internal/archive/archive.go` — reused as-is for the `Unpackage` step after download.

### Risks & Considerations

* **SSH key auth:** Verifying SSH key signatures requires `golang.org/x/crypto/ssh`, which is outside the current "stdlib + two compression libs" constraint. SSH key auth is descoped from this PRD. The `--auth token` mode (pre-shared bearer token) provides a comparable level of LAN security without a new dependency. SSH key auth can be added in a follow-up PRD once the dependency question is decided.
* **Large file streaming:** `http.ServeContent` handles this correctly (range requests, no full buffering). Ensure the manifest stores the `os.FileInfo` reference so `ServeContent` can set `Content-Length` and `Last-Modified` correctly.
* **Concurrent manifest refresh:** The background scanner must hold a `sync.RWMutex` when swapping the manifest map so concurrent readers are never served a half-written map.
* **Archive name collisions:** If two archives differ only by extension (e.g. `Doom.tar.xz` and `Doom.tar.gz`), only one will be served under the key `Doom`. Document this constraint; the server should log a warning and keep the first one found (alphabetical order).
* **Partial download cleanup:** The client must `defer os.Remove(tmpPath)` after a failed download and must not call `Unpackage` unless the full `Content-Length` bytes were received.
