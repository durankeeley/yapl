# YAPL (Yet Another Proton Launcher)

### Why I Made This

I originally built YAPL as I was setting up a bunch of PCs for a LAN party. I needed a way to get a game running perfectly on one machine, package it up completely, copy it to another machine, and have it **just work** (mostly).

YAPL tries solves the classic "it works on my machine" problem by treating your entire game setup as code. Every requirement, like the specific Proton build, DXVK version, and launch options, is defined in a simple JSON file. This lets you create reproducible, portable game environments that work the same way.

---

## How It Works

YAPL keeps things simple and organized.

* **A Single Binary**: The whole tool is a single `yapl` file compiled from Go. It has no external dependencies, so it runs on pretty much any modern Linux distro.
* **Shared Tools**: All your downloaded tools (like Proton and DXVK) are stored in one central place and cleverly reused across all your games and apps.
    * `./proton/`: Stores different Proton builds.
    * `./dependencies/`: Stores shared dependencies like DXVK, VKD33D, and `umu-launcher`.
* **Isolated Game Environments**: Each game gets its own clean Wine prefix and configuration, so they never interfere with each other.
    * `./games/`: Each subfolder in here contains a specific game's setup.
    * `./apps/`: Same structure, but for general-purpose applications.
* **Simple JSON Configs**:
    * `runner.json`: This is your global toolbox. It lists all the available versions of Proton and other tools, along with their download URLs.
    * `game.json` (or `app.json`): This local config sits inside each game's folder and tells YAPL exactly which tools and settings to use from the global `runner.json`.

---

## Quick Start 🚀

### 1. Initialize a Game Directory
This command creates the folder structure and a default `game.json` for you.

```bash
./yapl --game "My Awesome Game" setup
````

### 2\. Edit Your Configs

First, open the main `runner.json` file and add the download URLs for the Proton builds and other tools you want to use.

Next, open `games/My Awesome Game/game.json` and tell it which versions you want this specific game to use. You'll also need to set the path to your game's main executable here.

### 3\. Install & Package Your Game

Run the `setup` command again. This time, it will download all the components you just configured.

```bash
./yapl --game "My Awesome Game" setup
```

Now, install your game into the new prefix, usually somewhere inside `games/My Awesome Game/prefix/drive_c/`. Once it's installed and ready, you can package the entire environment into a single, portable archive.

```bash
./yapl --game "My Awesome Game" package --format xz
```

### 4\. Run the Game

That's it\! Now you can launch the game with all its dependencies perfectly configured.

```bash
./yapl --game "My Awesome Game" run
```

-----

## Command Reference

| Command | Description |
| :--- | :--- |
| `setup` | Creates the Wine prefix and downloads all defined dependencies. |
| `run` | Launches the application using the configured environment. |
| `package` | Compresses the entire game/app directory into a single `.tar` archive. |
| `unpackage` | Extracts one or more game/app archives into the appropriate directory. |

## Flags

| Flag | Description |
| :--- | :--- |
| `--game <name>` | Specifies the target game directory within `./games/`. |
| `--app <name>` | Specifies the target app directory within `./apps/`. |
| `--upgrade-proton` | Forces a re-download of the configured Proton version. |
| `--format <type>` | Sets the compression format for `package`. Options: `gz`, `xz`, `zst`. (Default: `gz`) |
| `--debug` | Enables verbose logging from Proton and DXVK. |
| `--steam` | Launches an isolated Steam client inside the prefix instead of the configured executable. |

```
```