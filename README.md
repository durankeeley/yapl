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
./yapl --game "Game" setup
```

### 2\. Edit Your Configs

First, open the main `runner.json` file and add the download URLs for the Proton builds and other tools you want to use. This file is created with placeholder values the first time you run YAPL.

Next, open `games/Game/game.json` and tell it which versions you want this specific game to use. You'll also need to set the path to your game's main executable here. (See examples below).

### 3\. Install & Package Your Game

Run the `setup` command again. This time, it will download all the components you just configured.

```bash
./yapl --game "Game" setup
```

Now, install your game into the new Wine prefix (when you first create a wine prefix it will open explorer), which is located at `games/Game/prefix/`. Once it's installed, you can package the environment.

```bash
# This creates a file like 'Game.tar.xz'
./yapl --game "Game" package --format xz
```

### 4\. Run the Game

That's it\! Now you can launch the game with all its dependencies perfectly configured.

```bash
./yapl --game "Game" run
```

To deploy on another machine, simply copy the `yapl` binary, your `runner.json`, the shared `dependencies` and `proton` folders, and the packaged game archive. Then run:

```bash
# This extracts the archive into the ./games/ directory
./yapl unpackage "Game.tar.xz"
```

-----

## Command Reference

| Command     | Description                                                                  |
| :---------- | :--------------------------------------------------------------------------- |
| `setup`     | Creates the Wine prefix and downloads all defined dependencies.             |
| `run`       | Launches the application using the configured environment.              |
| `package`   | Compresses the entire game/app directory into a single `.tar` archive.    |
| `unpackage` | Extracts one or more game/app archives into the appropriate directory (`games` or `apps`). |

## Flags

| Flag               | Description                                                                                                    |
| :----------------- | :------------------------------------------------------------------------------------------------------------- |
| `--game <name>`    | Specifies the target game directory within `./games/`.                                                        |
| `--app <name>`     | Specifies the target app directory within `./apps/`.                                                          |
| `--upgrade-proton` | Forces a re-download of the configured Proton version, even if it already exists.                             |
| `--format <type>`  | Sets the compression format for `package`. Options: `gz`, `xz`, `zst`. (Default: `gz`).                 |
| `--debug`          | Enables verbose logging from Proton and DXVK (`PROTON_LOG=1`, etc.).                                        |
| `--steam`          | A compatibility flag. It is **not** compatible with the `direct` launch method and is intended for container-based launches. |

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
  }
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
