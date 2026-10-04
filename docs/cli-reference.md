# YAPL CLI Reference

<!-- Generated from the command registrations. Regenerate with: go test ./cmd/yapl -run TestCLIReference -update -->

Usage: `yapl [global flags] <command> [arguments] [flags]`

## Global flags

Place these before the command.

| Flag | Description | Default |
|------|-------------|---------|
| `--workdir <path>` | Library path to use instead of the current directory (default $YAPL_HOME). |  |

## `yapl help`

Show help for YAPL or for one command

```
yapl help [command]
```

```bash
yapl help
yapl help run
```

## `yapl version`

Show the version of YAPL and each of its modules

```
yapl version [flags]
```

Every part of YAPL is versioned independently. LAN servers and clients use these versions to decide what needs upgrading.

| Flag | Description | Default |
|------|-------------|---------|
| `--output <format>` | Output format: text or json. | `text` |

```bash
yapl version
yapl version --output json
```

## `yapl setup`

Create a prefix and download dependencies

```
yapl setup <game|app> <name> [flags]
```

Creates games/<name>/ (or apps/<name>/) with a default config when missing, downloads Proton and dependencies, and initialises the Wine prefix.

| Flag | Description | Default |
|------|-------------|---------|
| `--config <file>` | Config file name inside the game directory (default game.json or app.json). |  |
| `--debug` | Enable verbose Proton and Wine logging. |  |
| `--method <method>` | Launch method for a newly created config: direct, container, umu. |  |
| `--steam` | Run as a Steam client prefix, ignoring the configured executable (container method only). |  |
| `--upgrade-proton` | Force a fresh download of the configured Proton build. |  |

```bash
yapl setup game "Need for Speed Most Wanted"
yapl setup app "Notepad++" --method direct
```

## `yapl install`

Install a game from an installer or a folder and detect its executable

```
yapl install <game|app> <name> (--installer <file> | --from <dir>) [flags]
```

Sets the game up if needed, then runs the installer inside the prefix or copies a portable game folder to drive_c/Games/<name>. The new executable that best matches the name is written to the config, unless the config already names a custom one.

| Flag | Description | Default |
|------|-------------|---------|
| `--config <file>` | Config file name inside the game directory (default game.json or app.json). |  |
| `--debug` | Enable verbose Proton and Wine logging. |  |
| `--executable <path>` | Executable path relative to the prefix; skips detection. |  |
| `--from <directory>` | Portable game directory to copy into the prefix. |  |
| `--installer <file>` | Windows installer file to run inside the prefix. |  |
| `--method <method>` | Launch method for a newly created config: direct, container, umu. |  |
| `--steam` | Run as a Steam client prefix, ignoring the configured executable (container method only). |  |
| `--upgrade-proton` | Force a fresh download of the configured Proton build. |  |

```bash
yapl install game "Doom" --installer ~/Downloads/setup_doom.exe
yapl install game "Half-Life" --from /mnt/cdrom/Half-Life
yapl install game "Half-Life" --from ./hl --executable drive_c/Games/Half-Life/hl.exe
```

## `yapl run`

Launch a game or app

```
yapl run <name> [flags]
```

| Flag | Description | Default |
|------|-------------|---------|
| `--config <file>` | Config file name inside the game directory (default game.json or app.json). |  |
| `--debug` | Enable verbose Proton and Wine logging. |  |
| `--steam` | Run as a Steam client prefix, ignoring the configured executable (container method only). |  |
| `--upgrade-proton` | Force a fresh download of the configured Proton build. |  |

```bash
yapl run "Doom"
yapl run "HalfLife" --config cstrike.json --debug
```

## `yapl package`

Create a distributable archive with a manifest

```
yapl package <name> [flags]
```

Writes <name>.tar.<format>, plus <name>.manifest.json (title, version, SHA-256, required components) and the cover art when the game has one. metadata.json is created in the game directory if missing.

| Flag | Description | Default |
|------|-------------|---------|
| `--bundle-deps` | Bundle Proton and dependencies into the archive for offline machines. |  |
| `--config <file>` | Config file name inside the game directory (default game.json or app.json). |  |
| `--fetch-cover` | Download cover art from Steam when steam_app_id is set and the game has no cover. |  |
| `--format <format>` | Compression format: gz, xz, zst. | `xz` |
| `--output-dir <path>` | Directory path to write the archive and its sidecar files to. | `.` |
| `--version <version>` | Package version (MAJOR.MINOR.PATCH); saved to metadata.json. |  |
| `--yes` | Skip confirmation prompts. |  |

```bash
yapl package "Doom"
yapl package "Doom" --version 1.2.0 --output-dir ./dist
yapl package "Doom" --format zst --bundle-deps --yes
```

## `yapl unpackage`

Extract one or more archives

```
yapl unpackage <game|app> <file>... [flags]
```

Extracts each archive into games/ or apps/. Bundled Proton and dependencies are installed and registered in runner.json.

```bash
yapl unpackage game Doom.tar.xz
yapl unpackage game Doom.tar.xz Quake.tar.zst
```

## `yapl info`

Show readiness status

```
yapl info <name> [flags]
```

| Flag | Description | Default |
|------|-------------|---------|
| `--config <file>` | Config file name inside the game directory (default game.json or app.json). |  |

```bash
yapl info "Doom"
```

## `yapl list`

List all configured games and apps

```
yapl list
```

## `yapl clean`

Remove prefix, Proton, or dependency directories

```
yapl clean <name> [flags]
```

| Flag | Description | Default |
|------|-------------|---------|
| `--all` | Equivalent to --prefix --proton --deps. |  |
| `--config <file>` | Config file name inside the game directory (default game.json or app.json). |  |
| `--deps` | Delete DXVK and VKD3D for this game's configured versions. |  |
| `--prefix` | Delete the game or app prefix directory. |  |
| `--proton` | Delete the downloaded Proton version. |  |
| `--yes` | Skip the confirmation prompt. |  |

```bash
yapl clean "Doom" --prefix
yapl clean "Doom" --all --yes
```

## `yapl serve`

Start a LAN package server

```
yapl serve --packages-dir <path> [flags]
```

Serves packages from <path>/games and <path>/apps over the v1 HTTP API (plus the legacy /packages endpoints) and announces itself on the LAN by UDP broadcast. Versioned packages live in <path>/games/<name>/<version>/; flat <name>.tar.* files copied in by hand are served too, and the directory is rescanned every 30s.

| Flag | Description | Default |
|------|-------------|---------|
| `--allow-push` | Accept uploads from 'yapl push'. Combine with --auth password on shared networks. |  |
| `--auth <mode>` | Auth mode: open or password. | `open` |
| `--packages-dir <path>` | Directory containing packaged archives (path, required). |  |
| `--password <string>` | Password for password auth. |  |
| `--port <port>` | TCP and UDP port to listen and broadcast on. | `8471` |

```bash
yapl serve --packages-dir /srv/games
yapl serve --packages-dir /srv/games --auth password --password lanparty --allow-push
```

## `yapl pull`

List or install packages from a YAPL server

```
yapl pull <list|name[:version]> [flags]
```

Installs name (latest version) or name:version. The archive and every component it needs (Proton, runtime, DXVK, …) are downloaded, checked against their SHA-256 and moved into place only when complete. The server is remembered in client.json; without --server YAPL uses that, or listens for a server's LAN broadcast.

| Flag | Description | Default |
|------|-------------|---------|
| `--auth <mode>` | Auth mode: open or password. | `open` |
| `--password <string>` | Password for password auth (default $YAPL_PASSWORD). |  |
| `--port <port>` | UDP port to listen on for discovery. | `8471` |
| `--server <address>` | Server address (host:port or URL). Auto-discovered when omitted. |  |

```bash
yapl pull list
yapl pull "Doom"
yapl pull "Doom:1.2.0"
yapl pull "Doom" --server 192.168.1.10:8471
```

## `yapl push`

Publish a game or app to a YAPL server

```
yapl push <name> [flags]
```

Packages the game, uploads every component (Proton, runtime, DXVK, VKD3D, umu-launcher) the server doesn't have yet, then uploads the package as <name>:<version>. Published versions are immutable; bump --version to publish again. The server must run with --allow-push.

| Flag | Description | Default |
|------|-------------|---------|
| `--auth <mode>` | Auth mode: open or password. | `open` |
| `--config <file>` | Config file name inside the game directory (default game.json or app.json). |  |
| `--format <format>` | Compression format for the package: gz, xz, zst. | `zst` |
| `--password <string>` | Password for password auth (default $YAPL_PASSWORD). |  |
| `--port <port>` | UDP port to listen on for discovery. | `8471` |
| `--server <address>` | Server address (host:port or URL). Auto-discovered when omitted. |  |
| `--version <version>` | Package version (MAJOR.MINOR.PATCH); defaults to metadata.json's. |  |

```bash
yapl push "Doom" --version 1.0.0
yapl push "Doom" --version 1.1.0 --server 192.168.1.10:8471 --auth password --password lanparty
```

## `yapl upgrade`

Upgrade only the parts that changed: games, yapl itself, or the defaults catalog

```
yapl upgrade [name[:version] | self | catalog] [flags]
```

Without arguments, lists what is out of date. With a name, installs the server's newer version of that game and keeps its saves (prefix/drive_c/users, prefix/user.reg and metadata.json "preserve" paths); an explicit :version can roll back. 'self' replaces this yapl binary with the server's when the server's is newer in some module and older in none. 'catalog' adds the newer pinned Proton, runtime, DXVK and VKD3D versions to runner.json. --all upgrades every outdated game plus the catalog.

| Flag | Description | Default |
|------|-------------|---------|
| `--all` | Upgrade every outdated game and the catalog. |  |
| `--auth <mode>` | Auth mode: open or password. | `open` |
| `--password <string>` | Password for password auth (default $YAPL_PASSWORD). |  |
| `--port <port>` | UDP port to listen on for discovery. | `8471` |
| `--server <address>` | Server address (host:port or URL). Auto-discovered when omitted. |  |

```bash
yapl upgrade
yapl upgrade "Doom"
yapl upgrade "Doom:1.0.0"
yapl upgrade --all
yapl upgrade self
yapl upgrade catalog
```

