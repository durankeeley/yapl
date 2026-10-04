## Why YAPL Exists

I originally built YAPL as I was setting up a bunch of PCs for a LAN party. I needed a way to get a game running on one machine, package it up, copy it to another machine and have it **work** (mostly).

YAPL solves the classic "it works on my machine" problem by treating your entire game setup as code. Every requirement like the specific Proton build, DXVK version, and launch options, are defined in a JSON file. Creating reproducible and portable game environments.

-----

## How It Works

YAPL keeps things simple and organized.

  * **A Single Binary**: The whole tool is a single `yapl` file compiled from Go. It has no external dependencies, so it runs on pretty much any modern Linux distribution.
  * **Shared Tools**: All your downloaded tools (like Proton, DXVK, and the Steam Runtime) are stored in one central place and cleverly reused across all your games and apps.
      * `./proton/`: Stores different Proton builds.
      * `./dependencies/`: Stores shared dependencies like `DXVK`, `VKD3D`, `umu-launcher`, and the Steam Linux Runtime.
  * **Isolated Game Environments**: Each game gets its own clean Wine prefix and configuration, so they never interfere with each other.
      * `./games/`: Each subfolder in here contains a specific game's setup.
      * `./apps/`: Same structure, but for general-purpose applications.
  * **Simple JSON Configs**:
      * `runner.json`: This is your global toolbox. It lists all the available versions of Proton and other tools, along with their download URLs or local paths.
      * `game.json` (or `app.json`): This local config sits inside each game's folder and tells YAPL exactly which tools and settings to use from the global `runner.json`.

-----

## Quick Start

### 1\. Set up a game

```bash
./yapl setup game "Game"
```

On a fresh library this works without editing anything. YAPL creates:

* `runner.json` with a pinned, known-good set of components: GE-Proton11-7, the Steam Linux Runtime (sniper), DXVK 3.1.1 and VKD3D-Proton 3.0.1. `./yapl version` shows the catalog version these pins come from.
* `games/Game/game.json` using those defaults and the `container` launch method.

It then downloads the components and creates the Wine prefix at `games/Game/prefix/`.

### 2\. Install the game

Point `install` at the game's installer, or at a folder for portable games (most CD-era LAN games):

```bash
./yapl install game "Game" --installer ~/Downloads/setup_game.exe
./yapl install game "Half-Life" --from /mnt/cdrom/Half-Life
```

`install` runs `setup` first if the prefix doesn't exist yet. It then runs the installer inside the prefix, or copies the folder to `drive_c/Games/<name>`, and finds the executable it added. Uninstallers, redistributables and crash reporters are skipped, and the remaining executables are ranked by how well their names match the game. The best match is written to `executable` in `game.json` and the ranked list is printed:

```
Detected executables (best match first):
  * drive_c/Games/Half-Life/hl.exe
    drive_c/Games/Half-Life/hlds.exe
✅ Set executable to drive_c/Games/Half-Life/hl.exe in games/Half-Life/game.json
```

If `game.json` already names a custom executable it is left alone. Pass `--executable <path>` to choose one yourself.

### 3\. Package it

```bash
# Creates 'Game.tar.xz' (xz is the default)
./yapl package "Game"
```

### 4\. Run the Game

```bash
./yapl run "Game"
```

YAPL automatically finds the game in `games/` or `apps/` — you never need to specify the type for `run`, `package`, or `info`.

**Online deployment** — copy the `yapl` binary, `runner.json`, the shared `dependencies/` and `proton/` folders, and the archive. Then run:

```bash
./yapl unpackage game "Game.tar.xz"
```

**Offline / LAN deployment** — use `--bundle-deps` when packaging so all dependencies are included in the archive. On the target machine you only need `yapl` and the archive:

```bash
# source machine
./yapl package "Game" --bundle-deps --yes

# target machine (no internet needed)
./yapl unpackage game "Game.tar.xz"
./yapl run "Game"
```

-----

## Command Reference

| Command                        | Description                                                                      |
| :----------------------------- | :------------------------------------------------------------------------------- |
| `setup game\|app <name>`       | Creates the Wine prefix and downloads all defined dependencies.                  |
| `install game\|app <name>`     | Installs from `--installer <file>` or `--from <dir>` and detects the executable.  |
| `run <name>`                   | Launches the application using the configured environment.                       |
| `package <name>`               | Compresses the game/app directory into a `.tar` archive (default: `.tar.xz`).   |
| `unpackage game\|app <files>`  | Extracts one or more archives into `games/` or `apps/`.                          |
| `list`                         | Lists all configured games and apps in the current working directory.            |
| `info <name>`                  | Displays the resolved configuration and readiness status.                        |
| `clean <name>`                 | Removes the prefix, Proton, or dependency directories for a game or app.         |
| `serve`                        | Starts a LAN package server that hosts packaged archives for clients to pull.    |
| `pull --server <addr> list`    | Lists packages available on a YAPL server.                                       |
| `pull --server <addr> <name>`  | Downloads and unpackages a game from a YAPL server.                              |
| `push <name>`                  | Publishes a game and any components the server lacks.                            |
| `upgrade [name\|self\|catalog]` | Upgrades only the parts whose version changed.                                   |
| `version`                      | Shows the version of the kernel and each module.                                 |
| `help [command]`               | Shows all commands, or one command's flags and examples.                         |

For `run`, `package`, and `info`, YAPL automatically finds the entry in `games/` or `apps/` — you never need to specify the type. Only `setup` and `unpackage` require it because they need to know where to create or extract.

### `list` — discover what's installed

```bash
./yapl list | column -t
```

Output:
```
TYPE  NAME       PROTON        METHOD     EXECUTABLE
game  Doom       ge-proton-9   direct     drive_c/Games/Doom/doom.exe
app   SteamCMD   system        container  drive_c/steamcmd/steamcmd.exe
```

`list` scans `games/` and `apps/` and prints a tab-separated summary. Pipe to `column -t` for aligned columns.

### `info` — check readiness before a LAN event

```bash
./yapl info "Doom"
```

Output:
```
Game:          Doom
Config file:   games/Doom/game.json
Proton:        ge-proton-9    [proton/ge-proton-9]  ✓ present
DXVK:          2.3            [dependencies/dxvk/2.3]  ✓ present
VKD3D:         (not set)
Runtime:       (not set)
Prefix:        games/Doom/prefix  ✓ initialised
Launch method: direct
Executable:    drive_c/Games/Doom/doom.exe
```

### `clean` — remove YAPL-managed directories

Free up disk space or reset a broken prefix without touching any other game's files.

```bash
# Delete only this game's Wine prefix (game.json is kept)
./yapl clean "Doom" --prefix

# Delete the downloaded Proton build (warns if other games share it)
./yapl clean "Doom" --proton

# Delete DXVK and VKD3D for the versions this game uses
./yapl clean "Doom" --deps

# Delete everything at once
./yapl clean "Doom" --all

# Skip the confirmation prompt in scripts
./yapl clean "Doom" --all --yes
```

### `serve` — host a LAN registry

The server works like a local Docker Hub for your games. It keeps every version of every package, with title, description and cover art, and clients pull them by name and tag.

```
/srv/games/
  games/
    Doom/
      1.0.0/  Doom.tar.xz  manifest.json  cover.jpg
      1.1.0/  Doom.tar.xz  manifest.json  cover.jpg
    Quake.tar.gz                 ← drop-in archive, served as version 0.0.0
    Half-Life.tar.xz             ← drop-in with sidecars from `yapl package`
    Half-Life.manifest.json
    Half-Life.cover.jpg
  apps/
    SteamCMD.tar.xz
```

Versioned folders are written by `yapl push` (see [Publishing to the server](#push--publish-to-a-lan-registry)). You can still copy archives in by hand. Copy the `.manifest.json` and `.cover.*` files that `yapl package` writes alongside them, and the server shows their title, version and art. Without sidecars a drop-in archive is listed as version `0.0.0` with no checksum.

```bash
# Open (no auth) — recommended for a trusted LAN
./yapl serve --packages-dir /srv/games

# Password-protected
./yapl serve --packages-dir /srv/games --auth password --password "lanparty"

# Custom port (default is 8471)
./yapl serve --packages-dir /srv/games --port 9000
```

The server rescans the directory every 30 seconds, so new packages show up without a restart. It broadcasts its address over UDP every 2 seconds so clients can find it automatically.

#### Registry API

| Method | Path | Returns |
|---|---|---|
| GET | `/v1/info` | API version, kernel and module versions, auth mode. Always public. |
| GET | `/v1/packages` | Catalog: name, type, title, description, tags, players, latest, versions, size, cover URL |
| GET | `/v1/packages/{name}` | Every version's manifest, newest first |
| GET | `/v1/packages/{name}/{version}/manifest` | One manifest. `latest` resolves to the highest version. |
| GET | `/v1/packages/{name}/{version}/archive` | The archive, with the `X-Yapl-SHA256` header. Supports `Range`. |
| GET | `/v1/packages/{name}/{version}/cover` | Cover image |
| GET | `/v1/components` | Every stored component (Proton builds, runtimes, DXVK, VKD3D, umu-launcher) |
| GET/HEAD | `/v1/components/{kind}/{version}/manifest` | A component's manifest (`HEAD` checks existence) |
| GET | `/v1/components/{kind}/{version}/archive` | The component archive (`.tar.zst`) |
| PUT | `/v1/packages/{name}/{version}` | Upload a package (multipart: `archive`, `cover`, `manifest`). Needs `--allow-push`. |
| PUT | `/v1/components/{kind}/{version}` | Upload a component (multipart: `archive`, `manifest`). Needs `--allow-push`. |
| GET | `/v1/yapl/binary` | The server's own `yapl` binary (with `X-Yapl-SHA256`), used by `yapl upgrade self` |
| GET | `/packages`, `/packages/{name}` | Legacy endpoints for older YAPL clients (latest version only) |

Names and versions that aren't safe path segments (`..`, `/`, a leading `.`) are rejected with `400`.

### `push` — publish to a LAN registry

`push` is YAPL's `docker push`. It packages the game, uploads any **components** the server doesn't have yet, then uploads the game as `<name>:<version>`:

```bash
# on the server
./yapl serve --packages-dir /srv/games --auth password --password lanparty --allow-push

# on the machine where you set the game up
./yapl push "Doom" --version 1.0.0 --server 192.168.1.10:8471 --auth password --password lanparty
# -> Uploading component proton/GE-Proton11-7 from proton/GE-Proton11-7
# -> Server already has runtime/sniper
# -> Uploading Doom:1.0.0
# ✅ Published Doom:1.0.0 (1 components uploaded, 1 already on the server)
```

Components are shared layers: Proton builds, Steam runtimes, DXVK, VKD3D and umu-launcher are stored once under `components/<kind>/<version>/`, however many games use them. Ten games on GE-Proton11-7 store and transfer that build once instead of ten times, which `--bundle-deps` can't do.

* Published versions can't be changed. Pushing `Doom:1.0.0` again fails, so bump `--version`.
* The server hashes every upload and rejects it if the SHA-256 doesn't match the manifest. Half-finished uploads are never visible.
* `push` packages with `zstd` by default, which compresses multi-GB games much faster than `xz`. Use `--format` to change it.
* Push is off unless the server runs with `--allow-push`. With `--auth open` the server warns that anyone on the LAN can publish.

### `pull` — install a game from a YAPL server

Name the game, with an optional version tag like Docker. The server says whether it's a game or an app.

```bash
./yapl pull list                 # catalog: name, type, latest version, title, size
./yapl pull "Doom"               # latest version
./yapl pull "Doom:1.2.0"         # a specific version
./yapl pull "Doom" --server 192.168.1.10:8471
YAPL_PASSWORD=lanparty ./yapl pull "Doom" --auth password
```

A pull leaves either a ready-to-play game or nothing:

1. The game's manifest names the components it needs. Any missing Proton build, runtime, DXVK or VKD3D is downloaded from the server and registered in `runner.json` (entries you already have are never changed). A component the server doesn't have is fine if `runner.json` can download it on first run; otherwise the pull stops before touching anything.
2. The archive streams through a SHA-256 check while it is extracted into `.staging/`. If the checksum doesn't match, the staging directory is deleted and nothing is installed.
3. The verified game is moved into `games/<name>` in one step, and `games/<name>/installed.json` records the version and server for `yapl upgrade`.

Pulling a game that's already installed tells you to use `yapl upgrade` instead. The server you pull from is saved in `client.json` (without the password), so later `pull`, `push` and console sessions reuse it without waiting for discovery. If it stops answering, YAPL falls back to discovery. Older YAPL servers without the v1 API still work: YAPL falls back to their legacy endpoints and says that the download can't be verified.

Every download (Proton, runtime, DXVK and packages) shows progress such as `Downloading GE-Proton11-7-x86_64.tar.gz: 412.0 MB / 480.3 MB (85.8%)`. When output goes to a file or pipe, YAPL writes one line per 10% instead of redrawing the line.

Auto-discovery listens for the server's UDP broadcast on port 8471 for up to 3 seconds. If discovery times out, pass `--server` to give the address directly. `--password` falls back to the `YAPL_PASSWORD` environment variable.

### `upgrade` — update only what changed

Every part of YAPL has its own version: each game, each component, each module of the `yapl` binary, and the defaults catalog. `upgrade` moves only the parts that changed:

```bash
./yapl upgrade                # what's out of date?
# PART     NAME         CURRENT       AVAILABLE
# game     Doom         1.0.0         1.1.0
# yapl     yapl         store 2.0.0   store 2.1.0
# catalog  runner.json  2026.04.0    2026.10.0

./yapl upgrade "Doom"         # newer Doom, keeping saves
./yapl upgrade "Doom:1.0.0"   # roll back to a specific version
./yapl upgrade --all          # every outdated game, plus the catalog
./yapl upgrade self           # replace this yapl with the server's newer build
./yapl upgrade catalog        # add newer pinned Proton/runtime/DXVK/VKD3D to runner.json (no server needed)
```

* **Games** download and verify the new version first, only installing it when complete. Your saves move across: `prefix/drive_c/users/`, `prefix/user.reg`, and any paths listed under `preserve` in `metadata.json` (relative to the game directory, globs allowed). The old version is deleted only after the new one is in place. If anything fails, the old install is left untouched. Components the new version needs are pulled automatically, and old ones stay for other games.
* **`self`** compares the server's per-module versions with yours. It upgrades only when the server is newer in at least one part and older in none, and refuses a binary built for a different OS or CPU. The server offers its own binary at `/v1/yapl/binary`, so one updated server can update every LAN machine.
* **`catalog`** adds pins without removing your entries and switches the `defaults` block to the new pins. Existing games keep the exact versions in their `game.json`; only new games use the new defaults.

### Package metadata and manifests

Every game directory can carry a `metadata.json`. It shows up in the LAN server's catalog and the console, and it drives upgrades. `package` creates it on first use with the title set to the game's name and version `1.0.0`:

```json
{
  "title": "Half-Life",
  "version": "1.1.0",
  "description": "Gordon Freeman's first day at Black Mesa.",
  "tags": ["fps", "classic"],
  "players": "1-32",
  "cover": "cover.jpg",
  "preserve": ["cstrike/config.cfg", "valve/save/*"]
}
```

* `version` must be `MAJOR.MINOR.PATCH`. Set it while packaging with `--version 1.2.0`.
* `cover` defaults to `cover.jpg`, `cover.png` or `cover.webp` in the game directory. `--fetch-cover` downloads Steam's library art when `steam_app_id` is set and there's no cover yet.
* `preserve` lists files, relative to the game directory, that upgrades must keep (saves and configs stored next to the game).

`package` writes three files to `--output-dir` (default: the current directory):

```
Doom.tar.xz            the archive
Doom.manifest.json     name, type, metadata, archive SHA-256 and size, required components
Doom.cover.jpg         the cover art, when the game has one
```

The manifest's `components` list names the Proton build, runtime, DXVK, VKD3D and umu-launcher versions the game needs, so a server or client can provide them separately from the game.

### `package --bundle-deps` — offline / LAN portability

Bundle Proton and all configured dependencies inside the archive so the target machine needs no internet access:

```bash
./yapl package "Doom" --bundle-deps
# Warning: bundled package will be approximately 4.2 GB. Continue? [y/N]: y
```

On the target machine, just run `yapl unpackage` as normal — Proton and dependencies are installed automatically from the bundle:

```bash
./yapl unpackage game Doom.tar.xz
# -> Installing bundled 'ge-proton-9'...
# -> Registered bundled proton/ge-proton-9 in runner.json
```

The bundle's `runner.json` entries are merged into the target machine's `runner.json`. Versions the machine doesn't know yet are added; versions it already defines are left alone. Pulling several bundled games onto one machine therefore works without editing anything.

Use `--yes` to skip the size confirmation prompt in scripts.

### Using system Wine

Set `"proton_version": "system"` in `game.json` to use whatever `wine64` or `wine` is in your `PATH`. No entry in `runner.json` is needed.

```json
{
  "proton_version": "system",
  "launch_method": "direct",
  "executable": "drive_c/steamcmd/steamcmd.exe"
}
```

### Winetricks

Add a `"winetricks"` array to `game.json` to install Windows redistributables into the prefix during `setup` and `run`:

```json
{
  "proton_version": "ge-proton-9",
  "launch_method": "direct",
  "executable": "drive_c/Games/MyGame/game.exe",
  "winetricks": ["vcrun2022", "dotnet48"]
}
```

`winetricks` must be installed and in your `PATH`.

YAPL reads `prefix/winetricks.log` before each run and skips any packages already recorded there, so repeated `run` or `setup` calls only install what is actually missing.

### Config overrides — multiple setups per game

Use `--config <file>` to load a different JSON config from the game's directory instead of the default `game.json`. All configs in the same directory share the same Wine prefix, so you can have several launch profiles without re-installing anything.

This is useful when a game has multiple mods or executables that need different settings. For example, Counter-Strike 1.5 alongside the base Half-Life install:

```
games/HalfLife/
  game.json          ← default: launches hl.exe
  cstrike.json       ← Counter-Strike 1.5: launches hl.exe -game cstrike
  dod.json           ← Day of Defeat: launches hl.exe -game dod
  prefix/            ← shared Wine prefix used by all three configs
```

`cstrike.json`:

```json
{
  "proton_version": "ge-proton-9",
  "launch_method": "direct",
  "executable": "drive_c/Games/HalfLife/hl.exe",
  "launch_args": ["-game", "cstrike", "-steam", "-nointro"]
}
```

Run a specific config:

```bash
./yapl run "HalfLife" --config cstrike.json
./yapl run "HalfLife" --config dod.json
./yapl setup game "HalfLife" --config cstrike.json
```

`setup` with `--config` will create the named file if it does not exist, using the same defaults as a normal `setup`. The prefix is still initialised in `games/HalfLife/prefix/` regardless of which config file is used.


### Background services

Some games need a helper process running before they start, such as a LAN relay for StarCraft Remastered. List these in `background_services`, with paths relative to the prefix like `executable`. YAPL starts each one with the same Wine/Proton environment, waits `background_service_delay_ms` (default `1000`), launches the game, and stops the services when the game exits.

```json
{
  "executable": "drive_c/Program Files (x86)/StarCraft/StarCraft.exe",
  "background_services": ["drive_c/Tools/lan-relay/relay.exe"],
  "background_service_delay_ms": 2000
}
```

---

### Config validation

`setup`, `run` and `package` check `game.json` before doing any work and report every problem at once:

* `proton_version` and `executable` are required.
* `launch_method` must be `direct`, `container` or `umu`; when it's omitted, `container` is assumed, and `container` needs `runtime_version`.
* `dependencies.dxvk_directx_version` must be `"9"`, `"10"` or `"11"` when set.

### Installing DXVK DLLs next to the game

Some old games load `d3d9.dll`/`d3d11.dll` from their own folder instead of the system. Set `dxvk_mode` to `custom` and YAPL copies the right DXVK DLLs (and VKD3D's `d3d12.dll` when `vkd3d_install_path` is set) into that folder during `setup`:

```json
"dependencies": {
  "dxvk_version": "3.1.1",
  "dxvk_mode": "custom",
  "dxvk_directx_version": "9",
  "dxvk_install_path": "Games/Need for Speed"
}
```

| `dxvk_directx_version` | DLLs copied |
|---|---|
| `9`  | `d3d9.dll` |
| `10` | `d3d10.dll`, `d3d10_1.dll`, `d3d10core.dll`, `d3d11.dll`, `dxgi.dll` |
| `11` | `d3d11.dll`, `dxgi.dll` |

## Running from anywhere — `--workdir` and `YAPL_HOME`

YAPL treats a folder as a *library*: `runner.json`, `proton/`, `dependencies/`, `games/` and `apps/` all live in it. By default the library is the current directory. To use one from anywhere (scripts, desktop launchers, systemd units), either pass `--workdir` **before** the command or set `YAPL_HOME`. The flag wins when both are set.

```bash
yapl --workdir /opt/lan-games run "Doom"
YAPL_HOME=/opt/lan-games yapl list
```

```ini
[Desktop Entry]
Name=Doom
Exec=yapl --workdir /opt/lan-games run Doom
Type=Application
```

## Flags

Every flag belongs to one command, and each command only accepts its own flags. To see them:

```bash
./yapl help              # all commands
./yapl help package      # one command's flags and examples
./yapl package --help    # same thing
```

The full reference, generated from the same source as `yapl help`, is in [`docs/cli-reference.md`](docs/cli-reference.md).

## Versions

YAPL is made of independently versioned modules (`lifecycle`, `registry`, `store`, …) on top of a small kernel. `./yapl version` lists them, and `./yapl version --output json` gives the same list as JSON for scripts. Servers and clients use these versions to work out which parts need an upgrade.

-----

## Configuration Examples

### `runner.json` (Global Toolbox)

A new library gets a working `runner.json` automatically (see Quick Start). The `defaults` block names the versions that new game and app configs are created with. When it's missing, YAPL picks the alphabetically first version of each kind.

```json
"defaults": { "proton": "GE-Proton11-7", "runtime": "sniper", "dxvk": "3.1.1", "vkd3d": "3.0.1" }
```

This file defines all the tools you *can* use. You only need to define each version once. ld_library_path_components and wine_dll_path_components are optional and only needed if your Proton build has libraries in non-standard locations.

```json
{
  "proton_versions": {
    "cachyos-proton-10-slr": {
      "url": "https://github.com/CachyOS/proton-cachyos/releases/download/cachyos-10.0-20250906-slr/proton-cachyos-10.0-20250906-slr-x86_64_v3.tar.xz",
      "ld_library_path_components": [
          "files/lib/x86_64-linux-gnu",
          "files/lib/i386-linux-gnu"
      ],
        "wine_dll_path_components": [
          "files/lib/vkd3d",
          "files/lib/wine"
      ]
    },
    "local-wine": {
      "path": "/home/user/builds/wine-tkg-staging"
    }
  },
  "runtime_versions": {
    "sniper": {
      "url": "https://repo.steampowered.com/steamrt-images-sniper/snapshots/latest-container-runtime-public-beta/SteamLinuxRuntime_sniper.tar.xz",
      "check_for_updates": true
    }
  },
  "dependency_versions": {
    "dxvk": {
      "2.7.1": {
        "url": "https://github.com/doitsujin/dxvk/releases/download/v2.7.1/dxvk-2.7.1.tar.gz"
      }
    },
    "vkd3d": {
      "2.14.1": {
        "url": "https://github.com/HansKristian-Work/vkd3d-proton/releases/download/v2.14.1/vkd3d-proton-2.14.1.tar.zst"
      }
    },
    "umu-launcher": {
      "1.2.9": {
        "url": "https://github.com/Open-Wine-Components/umu-launcher/releases/download/1.2.9/umu-launcher-1.2.9-zipapp.tar"
      }
    }
  }
}
```

### `game.json` Example 1: Direct Launch (Simple)

This is the most lightweight method, ideal for older or less demanding non-Steam games. It uses Proton's Wine binary directly without the Steam Runtime.

```json
{
  "proton_version": "cachyos-proton-10-slr",
  "launch_method": "direct",
  "executable": "drive_c/Games/MyGame/MyGame.exe",
  "dependencies": {
    "dxvk_version": "2.3",
    "vkd3d_version": "2.12"
  },
  "environment_vars": {
    "DXVK_HUD": "fps"
  },
  "launch_args": [
    "fullscrean=true"
  ]
}
```

### `game.json` Example 2: Container Launch (Maximum Compatibility)

This method uses the Steam Linux Runtime for a sandboxed, highly compatible environment, just like Steam. It's best for modern games that may have complex dependencies. steam_app_id is optional but recommended for better compatibility with certain games that are in steam (protonfixes).

```json
{
  "proton_version": "cachyos-proton-10-slr",
  "runtime_version": "sniper",
  "launch_method": "container",
  "executable": "drive_c/Program Files (x86)/Steam/steamapps/common/My Steam Game/bin/game.exe",
  "steam_app_id": "400",
  "dependencies": {
    "dxvk_version": "2.3",
    "vkd3d_version": "2.12"
  }
}
```

### DLL Overrides

Use `dll_overrides` to control how Wine resolves specific DLLs. This sets `WINEDLLOVERRIDES` — each key is a DLL name (without `.dll`) and the value is the load order:

| Value | Meaning |
| :---- | :------ |
| `"n"` | Native (Windows) DLL only |
| `"b"` | Builtin (Wine) DLL only |
| `"n,b"` | Try native first, fall back to builtin |
| `"b,n"` | Try builtin first, fall back to native |
| `""` | Disable the DLL entirely |

```json
{
  "proton_version": "cachyos-proton-10-slr",
  "launch_method": "direct",
  "executable": "drive_c/Games/MyGame/MyGame.exe",
  "dll_overrides": {
    "d3d11": "n,b",
    "dxgi": "n,b",
    "mscoree": ""
  }
}
```

This is commonly used to force native DXVK/VKD3D DLLs (`d3d11`, `dxgi`, `d3d12`) instead of Wine's builtins, or to disable specific DLLs that cause problems.

### `game.json` Example 3: `umu-launcher` (GOG/Epic Games/All Others)

This method uses the `umu-launcher` helper to correctly initialize platform-specific APIs (like GOG Galaxy or EOS) for non-Steam games.

```json
{
  "proton_version": "cachyos-proton-10-slr",
  "launch_method": "umu",
  "executable": "drive_c/GOG Games/Cyberpunk 2077/bin/x64/Cyberpunk2077.exe",
  "dependencies": {
    "dxvk_version": "2.3"
  },
  "umu_options": {
    "version": "0.3.1",
    "store": "gog",
    "game_id": "1423049311"
  }
}
```
