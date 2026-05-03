# PRD-32 — Refine LAN Server: remove token auth, directory-based type, UDP discovery

**Status:** `[x] Complete`
**Priority:** High
**Size:** M
**Sprint:** —
**Tags:** `server`, `networking`, `lan`, `simplification`
**Created:** 2026-05-03

---

## 1. Problem & Context

* **Token vs Password are redundant:** Both auth modes take a pre-shared string and send it on every request (Basic auth vs bearer header). A user configuring a LAN server has no reason to choose one over the other. Two modes that do the same thing add surface area without value.
* **`game|app` type on `pull` is user-hostile:** The server knows whether a package is a game or an app because it stores them in `games/` and `apps/` subdirectories. Requiring the client to pass this type is redundant information that can be eliminated automatically.
* **`--server` flag is friction for LAN party use:** The stated goal is `yapl pull "Doom"` with no arguments. When the server is on an open LAN, the client should auto-discover it via UDP broadcast rather than requiring manual address entry.

## 2. Proposed Solution

1. **Remove `token` auth mode.** Auth is now `open` (no credentials) or `password` (HTTP Basic Auth). `--token` flag removed everywhere.
2. **Server organises packages in `<packages-dir>/games/` and `<packages-dir>/apps/` subdirectories.** The manifest `PackageEntry` gains a `Type` field (`"game"` or `"app"`). Clients no longer specify a type when pulling.
3. **UDP broadcast discovery.** The server periodically broadcasts a `YAPL:<port>` message on UDP to `255.255.255.255:<port>`. When `--server` is omitted from `pull`, the client listens for this broadcast (3-second timeout) and derives the server address from the sender IP and announced port.

## 3. Scope & Design Details

### Auth simplification

* Valid `--auth` values: `open`, `password`. Any other value returns an error.
* All `--token` flags and `token` code paths removed from server, client, and main.go.

### Directory-based package type

* Server scans `<packages-dir>/games/` for game archives and `<packages-dir>/apps/` for app archives.
* `PackageEntry` adds `Type string` (`"game"` or `"app"`).
* Both `/packages` list endpoint and `/packages/{name}` download endpoint return or serve from the correct subdir.
* If the same bare name exists in both subdirs, the server logs a warning and keeps the `games/` entry.
* Operators organise their packages-dir like:
  ```
  /srv/yapl-packages/
    games/
      Doom.tar.xz
    apps/
      SteamCMD.tar.xz
  ```

### UDP broadcast discovery

* **Protocol:** server sends `YAPL:<http-port>` as a UDP datagram to `255.255.255.255:<http-port>` every 2 seconds.
* **Client discovery:** `client.Discover(port, timeout)` listens on UDP `0.0.0.0:<port>`, reads the first `YAPL:…` datagram, extracts the HTTP port from the message, and combines the sender IP with that port to produce the server address (e.g. `192.168.1.10:8471`).
* When `yapl pull` receives no `--server` flag it calls `client.Discover(8471, 3s)` first. If discovery times out, it fatals with a helpful message.
* Discovery works regardless of auth mode — knowing a server exists on the LAN does not leak credentials.

### `pull` command changes

* `game|app` argument removed entirely.
* `client.Download` returns `(archivePath, entryType string, err error)`. `main.go` derives `targetDir = entryType + "s"` from the returned type.

## 4. Execution & Milestones

- [ ] Update `internal/server` tests (failing first): directory layout, Type field, no token, broadcast message format
- [ ] Update `internal/client` tests (failing first): Type field in PackageEntry, no token, discovery
- [ ] Update server implementation: remove token, scan subdirs, add Type, add UDP broadcast
- [ ] Update client implementation: remove token, add Type to PackageEntry, update Download signature, add Discover
- [ ] Update `cmd/yapl/main.go`: remove --token, auto-discover, remove game|app from pull
- [ ] Run `go test ./...` — all pass
- [ ] Update README and developer-guide
- [ ] Mark Complete, move to `prds/done/`

## 5. Technical Notes

### Key Files

* `internal/server/server.go` — remove token, scan subdirs, add Type, UDP broadcast goroutine
* `internal/server/server_test.go` — update helper, remove token tests, add Type assertions, add broadcast test
* `internal/client/client.go` — remove token, update PackageEntry, update Download return, add Discover
* `internal/client/client_test.go` — update helper, remove token test, update download tests, add discover test
* `cmd/yapl/main.go` — remove --token, auto-discover when --server absent, remove game|app from pull

### Risks & Considerations

* **UDP broadcast and OS firewall:** Some firewalls block outbound UDP broadcasts. Document this as a known constraint; users can still pass `--server` manually.
* **Multiple YAPL servers on the same LAN:** `Discover` returns the first server found. If two servers broadcast simultaneously, the client picks whichever packet arrives first. This is acceptable for LAN party use; add a note to the docs.
* **Discovery test isolation:** Testing UDP broadcast/discover end-to-end in a unit test requires careful port management. Export `DiscoverFrom(conn net.PacketConn, timeout)` so tests can inject a fake connection without binding a real port.
