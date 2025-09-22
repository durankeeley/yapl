# YAPL (Yet Another Proton Launcher)

## Why YAPL Exists

I was building devices for a LAN party and I needed a way to build and test a game on one machine, package it up completely, copy it to another machine, test again and have it work (mostly). 

YAPL fixes "it works on my machine" this by treating game setups as **environments as code**. Every requirement — Proton build, DXVK version, launch options — is locked into a simple JSON config. Combine that with **umu-launcher**, which brings a universal runtime and a huge patch library, and you get portable game environments that are reproducible across any machine.

---

## How It Works

YAPL keeps things simple and organised:

* **Runner**: One self-contained Go binary. No external dependencies, runs on almost any Linux distro.
* **Shared Components**:

  * `./proton/` for Proton builds (downloaded once, reused everywhere)
  * `./dependencies/` for DXVK, VKD3D, umu-launcher, etc.
* **Game Prefixes**:

  * `./games/` and `./apps/` each hold prefixes and configs.
* **Configs as Code**:

  * `runner.json` defines available Proton and dependencies.
  * `game.json` defines what each game actually uses.

---

## Example: runner.json

```json
{
  "proton_versions": {
    "ge-proton-8": {
      "url": "https://github.com/GloriousEggroll/wine-ge-custom/releases/download/GE-Proton8-26/wine-lutris-GE-Proton8-26-x86_64.tar.xz",
      "bin_path_in_archive": "files/bin"
    },
    "cachyos-proton-10": {
      "url": "https://github.com/CachyOS/proton-cachyos/releases/download/cachyos-10.0-20250906-slr/proton-cachyos-10.0-20250906-slr-x86_64_v3.tar.xz",
      "bin_path_in_archive": "files/bin"
    }
  },
  "dependency_versions": {
    "umu-launcher": {
      "1.2.9": {
        "url": "https://github.com/Open-Wine-Components/umu-launcher/releases/download/1.2.9/umu-launcher-1.2.9-zipapp.tar"
      }
    },
    "dxvk": {
      "v2.7.1": {
        "url": "https://github.com/doitsujin/dxvk/releases/download/v2.7.1/dxvk-2.7.1.tar.gz"
      }
    },
    "vkd3d": {
      "v2.14.1": {
        "url": "https://github.com/HansKristian-Work/vkd3d-proton/releases/download/v2.14.1/vkd3d-proton-2.14.1.tar.zst"
      }
    }
  }
}
```

---

## Example: game.json

```json
{
  "proton_version": "cachyos-proton-10",
  "executable": "drive_c/UnrealTournament/System/UnrealTournament.exe",
  "use_umu_launcher": true,
  "umu_options": {
    "version": "1.2.9",
    "use_system_binary": false,
    "game_id": "ut99",
    "store": "local",
    "launch_args": [
      "-opengl",
      "-SkipBuildPatchPrereq"
    ]
  },
  "dependencies": {
    "dxvk_mode": "proton"
  },
  "dll_overrides": {
    "dinput8": "n,b"
  },
  "environment_vars": {}
}
```

---

## Understanding `dxvk_mode`

This key controls how DirectX gets translated to Vulkan.

* **`custom` (default)**

  * Uses exact DXVK and VKD3D versions from your global `runner.json`
  * Installs them into the game directory during setup
  * Best when a game needs a very specific version

* **`proton`**

  * Uses DXVK and VKD3D bundled in your chosen Proton build
  * No custom downloads
  * Good for most games where Proton defaults are fine

* **`wined3d`**

  * Disables DXVK and VKD3D entirely
  * Uses Wine’s built-in D3D-to-OpenGL/Vulkan libraries
  * Useful for very old games or troubleshooting

---

## Quick Start

1. **Create a Game**

   ```sh
   mkdir -p games
   ./runner --game UnrealTournament setup
   ```

   This creates `games/UnrealTournament/game.json`.

2. **Edit runner.json**
   Define Proton builds and dependencies with their URLs.

3. **Edit game.json**
   Tell YAPL which Proton, dependencies, and options your game needs.

4. **Install and Package**

   ```sh
   ./runner --game UnrealTournament setup
   ./runner --game UnrealTournament package
   ```

5. **Run**

   ```sh
   ./runner --game UnrealTournament run
   ```

---

## Commands

| Command   | What it does                                              |
| --------- | --------------------------------------------------------- |
| `setup`   | Creates a new prefix, downloads deps, opens Wine Explorer |
| `run`     | Launches the game or app                                  |
| `package` | Creates a compressed tar.gz for easy sharing              |

Flags:

* `--game <name>`: work with a game in `./games/<name>`
* `--app <name>`: work with an app in `./apps/<name>`
* `--upgrade-proton`: force re-download of Proton

---

## Build From Source

```sh
cd /path/to/yapl
go get github.com/ulikunitz/xz
go get github.com/klauspost/compress/zstd
go build -o runner .
```
