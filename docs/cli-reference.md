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

Create a distributable archive

```
yapl package <name> [flags]
```

| Flag | Description | Default |
|------|-------------|---------|
| `--bundle-deps` | Bundle Proton and dependencies into the archive for offline machines. |  |
| `--config <file>` | Config file name inside the game directory (default game.json or app.json). |  |
| `--format <format>` | Compression format: gz, xz, zst. | `xz` |
| `--yes` | Skip confirmation prompts. |  |

```bash
yapl package "Doom"
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

Serves archives from <path>/games and <path>/apps over HTTP and announces itself on the LAN by UDP broadcast.

| Flag | Description | Default |
|------|-------------|---------|
| `--auth <mode>` | Auth mode: open or password. | `open` |
| `--packages-dir <path>` | Directory containing packaged archives (path, required). |  |
| `--password <string>` | Password for password auth. |  |
| `--port <port>` | TCP and UDP port to listen and broadcast on. | `8471` |

```bash
yapl serve --packages-dir /srv/games
yapl serve --packages-dir /srv/games --auth password --password lanparty
```

## `yapl pull`

List or download packages from a YAPL server

```
yapl pull <list|name> [flags]
```

Without --server, YAPL listens for a server's LAN broadcast.

| Flag | Description | Default |
|------|-------------|---------|
| `--auth <mode>` | Auth mode: open or password. | `open` |
| `--output-dir <path>` | Keep the downloaded archive in this path (default: a temporary directory). |  |
| `--password <string>` | Password for password auth. |  |
| `--port <port>` | UDP port to listen on for discovery. | `8471` |
| `--server <address>` | Server address (host:port or URL). Auto-discovered when omitted. |  |

```bash
yapl pull list
yapl pull "Doom"
yapl pull --server 192.168.1.10:8471 "Doom"
```

