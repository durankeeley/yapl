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

### 1\. Initialize a Game Directory

This command creates the folder structure (`./games/Game/`) and a default `game.json` for you.

```bash
./yapl setup game "Game"
```

### 2\. Edit Your Configs

First, open the main `runner.json` file and add the download URLs for the Proton builds and other tools you want to use. This file is created with placeholder values the first time you run YAPL.

Next, open `games/Game/game.json` and tell it which versions you want this specific game to use. You'll also need to set the path to your game's main executable here. (See examples below).

### 3\. Install & Package Your Game

Run `setup` again. This time it will download all the components you configured.

```bash
./yapl setup game "Game"
```

Now install your game into the new Wine prefix (when you first create a Wine prefix it will open Explorer), which is located at `games/Game/prefix/`. Once it's installed, package the environment.

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
| `run <name>`                   | Launches the application using the configured environment.                       |
| `package <name>`               | Compresses the game/app directory into a `.tar` archive (default: `.tar.xz`).   |
| `unpackage game\|app <files>`  | Extracts one or more archives into `games/` or `apps/`.                          |
| `list`                         | Lists all configured games and apps in the current working directory.            |
| `info <name>`                  | Displays the resolved configuration and readiness status.                        |
| `clean <name>`                 | Removes the prefix, Proton, or dependency directories for a game or app.         |
| `serve`                        | Starts a LAN package server that hosts packaged archives for clients to pull.    |
| `pull --server <addr> list`    | Lists packages available on a YAPL server.                                       |
| `pull --server <addr> <name>`  | Downloads and unpackages a game from a YAPL server.                              |

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

### `serve` — host a LAN package server

Organise your packaged archives into `games/` and `apps/` subdirectories inside a packages directory, then run this on the host machine. Clients can discover the server automatically on the LAN and pull any game with a single command.

```
/srv/games/
  games/
    Doom.tar.xz
    NeedForSpeedMostWanted.tar.xz
  apps/
    SteamCMD.tar.xz
```

```bash
# Open (no auth) — recommended for a trusted LAN
./yapl serve --packages-dir /srv/games

# Password-protected
./yapl serve --packages-dir /srv/games --auth password --password "lanparty"

# Custom port (default is 8471)
./yapl serve --packages-dir /srv/games --port 9000
```

The server scans the subdirectories on startup and refreshes every 30 seconds, so you can add new packages without restarting. It also broadcasts its address via UDP every 2 seconds so clients can auto-discover it.

### `pull` — download a game from a YAPL server

On a client machine, just name the game. The server tells the client whether it's a game or app, so you never need to specify that yourself.

```bash
# Auto-discover the server and list available packages
./yapl pull list

# Auto-discover and download + unpackage a game
./yapl pull "Doom"

# Specify a server manually (useful when auto-discovery doesn't work across subnets)
./yapl pull --server 192.168.1.10:8471 "Doom"

# With password auth
./yapl pull --auth password --password "lanparty" "Doom"
```

Auto-discovery listens for the server's UDP broadcast on port 8471 for up to 3 seconds. If discovery times out, pass `--server` to specify the address directly.

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
# -> Installing bundled runner.json...
```

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

## Flags

| Flag                    | Description                                                                                                    |
| :---------------------- | :------------------------------------------------------------------------------------------------------------- |
| `--config <file>`       | Use a custom config file name (e.g. `mod-a.json`) instead of `game.json` / `app.json`.                       |
| `--method <type>`       | Set the launch method (`direct`, `container`, `umu`) when `setup` creates a new config. Ignored if the config file already exists. |
| `--upgrade-proton`      | Forces a re-download of the configured Proton version, even if it already exists.                             |
| `--format <type>`       | Compression format for `package`. Options: `gz`, `xz`, `zst`. (Default: `xz`).                              |
| `--bundle-deps`         | Bundles Proton and dependencies into the package for offline deployment. Use with `package`.                 |
| `--yes`                 | Skips confirmation prompts (e.g. the size warning for `--bundle-deps` or the `clean` confirmation).          |
| `--debug`               | Enables verbose logging from Proton and DXVK (`PROTON_LOG=1`, etc.).                                        |
| `--steam`               | A compatibility flag. It is **not** compatible with the `direct` launch method and is intended for container-based launches. |
| `--prefix`              | (`clean` only) Delete the game/app prefix directory.                                                         |
| `--proton`              | (`clean` only) Delete the downloaded Proton build for this game.                                             |
| `--deps`                | (`clean` only) Delete the DXVK and VKD3D directories for this game's configured versions.                   |
| `--all`                 | (`clean` only) Equivalent to `--prefix --proton --deps`.                                                     |
| `--packages-dir <path>` | (`serve` only) Directory whose `games/` and `apps/` subdirectories contain packaged archives. Required.     |
| `--port <int>`          | (`serve` only) Port to listen on. Default: `8471`.                                                           |
| `--auth <mode>`         | (`serve`/`pull`) Auth mode: `open` or `password`. Default: `open`.                                          |
| `--password <string>`   | (`serve`/`pull`) Shared password for `password` auth mode.                                                   |
| `--server <addr>`       | (`pull` only) YAPL server address (`host:port` or full URL). Omit to auto-discover via UDP broadcast.       |
| `--output-dir <path>`   | (`pull` only) Where to save the downloaded archive before unpackaging (default: system temp dir).            |

-----

## Configuration Examples

### `runner.json` (Global Toolbox)

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
