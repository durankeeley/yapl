# YAPL Developer Guide

A plain-language guide to understanding, building, and contributing to YAPL.

---

## Project Setup

### Clone and build

```bash
git clone <repository_url>
cd yapl
go mod tidy
go build -o yapl ./cmd/yapl/
```

The output binary has no external runtime dependencies. Copy it anywhere.

### Run the tests

```bash
go test ./...                        # all packages
go test -v ./internal/archive/      # verbose for one package
```

---

## What does YAPL actually do?

Imagine you set up a game on your PC. You downloaded a specific version of Proton (the compatibility layer that lets Windows games run on Linux), installed the game into a Wine prefix (a fake Windows environment), and tweaked a bunch of settings to make it work perfectly.

Now you want to run the same game on 10 other PCs at a LAN party. Without YAPL, you'd have to do all that setup on every machine. With YAPL, you:

1. Define everything (which Proton, which DXVK, which settings) in a JSON file
2. Package the game + its Wine prefix into a single `.tar` file
3. Copy that file and the `yapl` binary to another machine — or run `yapl pull` to fetch it from a YAPL server
4. Run `yapl unpackage` and then `yapl run` — done

YAPL's job is to make that workflow reliable and repeatable.

---

## The two config files

Everything YAPL does is driven by two JSON files:

### `runner.json` — The Global Toolbox

This lives in the same directory as the `yapl` binary. It lists all the tools YAPL can download and use. Think of it as a catalogue.

```json
{
  "proton_versions": {
    "ge-proton-9": { "url": "https://..." }
  },
  "runtime_versions": {
    "sniper": { "url": "https://..." }
  },
  "dependency_versions": {
    "dxvk": {
      "2.3": { "url": "https://..." }
    }
  }
}
```

You define versions once here. Games then reference versions by name.

### `game.json` (or `app.json`) — Per-Game Config

This lives inside each game's folder: `games/MyGame/game.json`. It says which tools from the catalogue this game needs and how to launch it.

```json
{
  "proton_version": "ge-proton-9",
  "launch_method": "direct",
  "executable": "drive_c/Games/MyGame/game.exe",
  "dependencies": {
    "dxvk_version": "2.3"
  }
}
```

---

## The folder structure

When you run YAPL for the first time, it creates and manages these folders:

```
./runner.json              <- your global catalogue
./yapl                     <- the binary
./proton/                  <- Proton builds live here (downloaded once, shared)
./dependencies/
    dxvk/                  <- DXVK versions
    vkd3d/                 <- VKD3D versions
    runtime/               <- Steam Linux Runtime
./games/
    MyGame/
        game.json          <- per-game settings
        prefix/            <- the Wine prefix (fake Windows install)
./apps/
    MyApp/
        app.json
        prefix/
```

Everything under `proton/` and `dependencies/` is **shared** across all games. You download each version once. Each game gets its own isolated `prefix/`.

---

## The commands

| Command | What it does |
|---------|-------------|
| `setup` | Downloads all dependencies for a game and creates its Wine prefix |
| `run` | Launches the game |
| `package` | Zips up a game folder into a `.tar.gz` / `.tar.xz` / `.tar.zst` file |
| `unpackage` | Extracts one of those archives back into `games/` or `apps/` |
| `list` | Lists all configured games and apps in the current directory |
| `info` | Shows readiness status for a specific game or app |
| `clean` | Removes the prefix, Proton, or dependency directories for a game |
| `serve` | Starts a LAN package server that hosts packaged archives |
| `pull` | Downloads and unpackages a game from a YAPL server |

---

## The three launch methods

`launch_method` in `game.json` controls how the game is actually started:

### `direct`
The simplest method. YAPL finds the `wine64` binary inside the Proton folder and calls it directly. No Steam Runtime involved. Good for older games and simple apps.

```
yapl binary → wine64 (from proton folder) → game.exe
```

### `container`
Uses the Steam Linux Runtime — a minimal Linux container that Steam itself uses. The game runs inside this container for maximum compatibility. Required for modern games with complex library dependencies. Requires `runtime_version` to be set in `game.json`.

```
yapl binary → Steam Runtime entry point → proton script → game.exe
```

### `umu`
Uses a helper called `umu-launcher` that knows how to set up platform-specific APIs (GOG, Epic, etc.) before launching the game through Proton.

```
yapl binary → umu-run → proton → game.exe
```

---

## How the code is organised

All the interesting code lives under `internal/`. Here's what each package does:

### Architecture: a microkernel with versioned modules

```
cmd/yapl/main.go ── registers ──► internal/kernel ◄── Module{Name, Version, Commands}
                                        │
              ┌─────────────────────────┼──────────────────────────┐
   internal/modules/lifecycle   internal/modules/registry   internal/modules/store
   setup run package unpackage  serve                       pull
   info list clean
```

* **`internal/kernel`** owns argument parsing, `help`, `version` and dispatch. Each command gets its own `flag.FlagSet`, so flags are scoped to their command and may appear before or after positional arguments. Usage text, `yapl help` and `docs/cli-reference.md` are all generated from the command registrations.
* **Modules** (`internal/modules/*`) are thin: they declare commands and flags, validate arguments, and call into the domain packages (`app`, `archive`, `client`, `server`, …). Each has `const Version`; bump it when the module's behaviour changes.
* **`internal/defaults`** holds every shared tunable (ports, timeouts, intervals, default format and method). Don't write a port or timeout literal anywhere else.
* **`internal/paths`** defines the library layout (`proton/`, `dependencies/<kind>/<version>`, `games/<name>/prefix`, `runner.json`, …) and the component kinds. Paths are relative to the library root; `--workdir`/`YAPL_HOME` change into that root before any command runs.
* **`internal/version`** parses and orders versions (semantic, plus natural order for strings like `GE-Proton11-7`).

### `cmd/yapl/main.go`
Registers the modules with the kernel and hands it `os.Args`. Nothing else.

### `internal/app/app.go`
The coordinator. It holds the loaded config and calls into the other packages in the right order. When you run `setup`, it calls `dependency.EnsureAll`, then `dependency.EnsureRuntime`, then `command.InitializePrefix`. When you run `run`, it does the same setup and then calls one of the launch methods. The `Clean` method handles removing YAPL-managed directories.

### `internal/config/config.go`
Knows how to read and write `runner.json` and `game.json`. If a config file doesn't exist, it creates a sensible default so the user has a starting point.

### `internal/dependency/`
`dependency.go` — Downloads Proton and optional dependencies (DXVK, VKD3D, umu-launcher) if they aren't already present. Checks whether a directory is non-empty before downloading.

`runtime.go` — Handles the Steam Linux Runtime specifically. It can check whether the local version matches the remote version and re-download if there's an update.

### `internal/command/command.go`
The most complex file. Contains:
- `buildProtonEnv` — Assembles all the environment variables that Proton needs (`WINEPREFIX`, `LD_LIBRARY_PATH`, `STEAM_COMPAT_*`, etc.)
- `RunDirectly` / `RunInContainer` / `RunWithUMU` — The three launch method implementations
- `InitializePrefix` — Creates a new Wine prefix if one doesn't exist

### `internal/archive/archive.go`
Handles all archive operations: download a `.tar.xz` from the internet and extract it, or create a `.tar.gz` from a directory. Supports `gz`, `xz`, and `zst` compression.

### `internal/fs/fs.go`
Simple filesystem helpers that are needed in more than one place: copy a file, copy a directory, check if a directory exists and isn't empty, resolve an absolute path.

### `internal/server/server.go`
The LAN package server. Scans `<packages-dir>/games/` and `<packages-dir>/apps/` for `*.tar.*` archives and caches a manifest in memory. Each `PackageEntry` carries a `Type` field (`"game"` or `"app"`) so clients know where to unpackage without being told explicitly. Serves `GET /packages` (JSON list) and `GET /packages/{name}` (file download via `http.ServeContent`). Supports `open` and `password` (HTTP Basic Auth) auth modes. The manifest refreshes every 30 seconds and the server broadcasts its presence via UDP every 2 seconds so clients can auto-discover it.

### `internal/client/client.go`
The LAN package client. Connects to a YAPL server to list available packages or download one. `Download` returns the archive path and the entry type (`"game"` or `"app"`) so the caller can extract to the right directory. `Discover`/`DiscoverFrom` listen for the server's UDP broadcast and return the server address. Handles auth headers, reports download progress, and cleans up partial files on failure.

---

## How a typical `run` flows through the code

Here's what happens when you type `./yapl run "Doom"`:

1. **`kernel`** — Matches the `run` verb to the lifecycle module and parses its flags. **`lifecycle`** calls `app.Find("Doom")` to locate it in `games/` or `apps/`, loads configs, creates an `App` struct and calls `app.Run()`.

2. **`app.Run()`** — Calls `dependency.EnsureAll()` to make sure Proton and DXVK are downloaded. Then calls `dependency.EnsureRuntime()` if a runtime is configured. Then calls `command.InitializePrefix()` to make sure the Wine prefix exists.

3. **`dependency.EnsureAll()`** — Checks if `proton/ge-proton-9/` is a non-empty directory. If not, downloads the tarball and extracts it. Same check for DXVK.

4. **`command.InitializePrefix()`** — Checks if `games/Doom/prefix/system.reg` exists (that file means the prefix is already initialised). If not, runs the Proton script to create it.

5. **`app.Run()`** continues — Looks at `launch_method` in `game.json` and calls the right function: `RunDirectly`, `RunInContainer`, or `RunWithUMU`.

6. **`RunDirectly()`** (for example) — Calls `buildProtonEnv()` to assemble all the environment variables, then `exec.Command(wineExecutablePath, ...)` to launch the game.

---

## How `install` flows through the code

1. **`lifecycle` module** validates `<game|app> <name>` and that exactly one of `--installer`/`--from` is given, then loads or creates the config.
2. **`app.Install`** runs `Setup` if the prefix isn't initialised, then takes a `detect.Snapshot` of every `.exe` under `drive_c` (excluding `drive_c/windows`).
3. `--from` copies the folder to `drive_c/Games/<name>`. `--installer` reuses `app.launch` with the installer as an absolute executable (runners accept absolute paths, and `steam_appid.txt` is never written next to an installer). In `direct` mode it then waits with `wineserver -w`, because installers often fork and return.
4. A second snapshot gives the new executables. **`detect.Candidates`** drops helpers (uninstallers, redistributables, crash reporters) and ranks the rest by full-name match, initials (`hl.exe` for Half-Life), word matches, then directory depth and file size. The weights are named constants in `internal/detect`.
5. The best match replaces the placeholder executable and the config is saved to `App.ConfigFile`, which honours `--config`.

---

## How a typical `pull` flows through the code

Here's what happens when you type `./yapl pull --server 192.168.1.10:8471 "Doom" game`:

1. **`kernel` → `store` module** — Parses the `pull` command and creates a `client.Client` with the server address and auth config.

2. **`client.List()`** — Fetches `GET /packages` to discover the filename extension for the `Doom` package.

3. **`client.Download()`** — Fetches `GET /packages/Doom`, writes the response to a temp file while reporting progress to stderr. Cleans up the temp file if the download fails.

4. **`archive.Unpackage()`** — Extracts the downloaded archive into `games/`.

---

## How to add a new command

1. Pick the module the command belongs to (or create `internal/modules/<name>` with `const Version` and a `Module` type, and register it in `cmd/yapl/main.go`).
2. Add a `kernel.Command` with `Verb`, `Usage`, `Summary`, `Examples` and a `Flags` function. Return `kernel.UsageError(...)` for bad arguments.
3. Put the real logic in a domain package under `internal/`, not in the module.
4. Write tests first (failing), then the implementation.
5. Regenerate the CLI reference: `go test ./cmd/yapl -run TestCLIReference -update`. The test fails until you do.
6. Bump the module's `Version`.

## How to add a new launch method

1. Add your new method function in `internal/command/command.go` (e.g., `RunWithNewMethod`)
2. Add a `case "newmethod":` in the `switch` in `app.Run()` in `internal/app/app.go`
3. Update the error message in the `default` case to mention the new method
4. Write tests

---

## Importing an existing prefix from Lutris

If you have already set up a game in Lutris and want to bring it into YAPL:

1. Find the Lutris prefix. It is usually somewhere under `~/.local/share/lutris/runners/wine/` or `~/Games/`.
2. Copy (or move) it to `games/YourGameName/prefix/`.
3. Create a `games/YourGameName/game.json` pointing at the executable (path relative to `prefix/`).
4. Run `./yapl run "YourGameName"`.

YAPL auto-detects both prefix layouts:
- **Flat layout** (YAPL standard): `system.reg` at `prefix/system.reg`
- **Lutris/raw Proton layout**: `system.reg` at `prefix/pfx/system.reg`

When the Lutris layout is detected, YAPL migrates it to the flat layout automatically by moving the files out of `pfx/` and creating a `pfx → .` symlink in its place. This happens once and is printed to the console.

## Using system Wine

Set `"proton_version": "system"` in `game.json` and omit `proton_version` from `runner.json` (or add an empty entry). YAPL will use `wine64` or `wine` from your `PATH` to create and run the prefix. Useful for quick testing but gives less isolation than a pinned Proton build.

## 32-bit games

Wine 11.0+ (January 2026) finalized WoW64 mode: a single 64-bit Wine process can run 32-bit and 16-bit Windows applications with no extra setup. All modern Proton builds (GE-Proton, CachyOS Proton, etc.) include this. You do not need to set any special config for 32-bit games — just point YAPL at the executable and it will work.

The old `wine_arch: win32` config option has been removed.

## Common gotchas

**Proton and DXVK are shared.** If you delete the `proton/` folder it affects every game. Use `yapl clean --proton` — it warns you if another game shares the same Proton version.

**The `container` method needs a full Proton build, not a Wine-only build.** It needs the `proton` script that GE-Proton and similar builds include. If you use a plain Wine build (e.g., `tkg-wine`), use `direct` instead.

**`runner.json` is the single source of truth for versions.** If a version key in `game.json` doesn't exist in `runner.json`, YAPL will error. Always define the version in `runner.json` first.

**Archive name collisions on the server.** If a packages directory contains both `Doom.tar.xz` and `Doom.tar.gz`, only one will be served. The server logs a warning. Use a single format per game to avoid this.

**Auto-discovery only works on the same subnet.** UDP broadcasts don't cross routers. If the client and server are on different subnets, pass `--server <addr>` manually.

**Multiple YAPL servers on the same LAN.** `pull` without `--server` picks the first broadcast received. If two servers are running simultaneously, the client may connect to either one.
