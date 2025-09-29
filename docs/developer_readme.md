# YAPL Developer README

This document provides an overview of the YAPL codebase, architecture, and development workflow. It is intended for anyone looking to understand, modify, or contribute to the project.

## 1\. Project Setup

To set up a local development environment, follow these steps:

### Clone the Repository

```bash
git clone <repository_url>
cd yapl
```

### Install Dependencies

The project uses Go Modules[cite: 1]. Dependencies will be fetched automatically on the first build. To fetch them manually:

```bash
go mod tidy
```

### Build the Binary

The main application entrypoint is located in `main.go` in the project root. To build the executable, run the following command:

```bash
go build -o yapl .
```

This will create a `yapl` executable in the project's root directory.

-----

## 2\. Architectural Overview

The code is divided into several private packages within the `/internal` directory. This layered architecture ensures that each part of the application has a single, well-defined responsibility.

The general flow of the application is as follows:

1.  **`main.go`**: The application entrypoint. It is responsible only for parsing command-line flags and dispatching commands to the correct handler.
2.  **`internal/app/app.go`**: The core orchestrator. The `main` function creates an `App` instance, which holds the application's state and configuration. High-level commands like `app.Run()` or `app.Setup()` are executed from here.
3.  **Specialized Packages**: The `App` struct delegates tasks to specialized packages:
      * `internal/config`: Handles loading, creating, and saving `runner.json` and `game.json`/`app.json` files.
      * `internal/dependency`: Manages the logic for downloading, extracting, and verifying Proton, DXVK, the Steam Runtime, and other tools.
      * `internal/command`: Responsible for all shell interactions, such as running `exec.Command`, building complex environment variable sets, and initializing Wine prefixes.
      * `internal/archive`: A utility package for creating and extracting various `.tar` archive formats (`.tar.gz`, `.tar.xz`, `.tar.zst`).
      * `internal/fs`: Contains simple, reusable filesystem helper functions.

-----

## 3\. Package & Function Deep Dive

### `internal/config`

  * **Purpose**: To define all configuration data structures (`Global`, `App`, etc.) and manage their serialization to/from JSON files.
  * **Key Functions**:
      * `LoadOrCreateGlobal()`: Reads `runner.json`. If it doesn't exist, it creates a default template to guide the user.
      * `LoadOrCreateApp()`: Reads the local `game.json` or `app.json`. If it doesn't exist, it creates one with sensible defaults.

### `internal/dependency`

  * **Purpose**: To ensure all required tools (Proton, DXVK, Steam Runtime, etc.) are downloaded and available in the shared directories.
  * **Key Functions**:
      * `EnsureAll()`: The main entrypoint for this package. It checks all dependencies listed in the app's config and triggers downloads if necessary.
      * `EnsureRuntime()`: Specifically handles the logic for downloading, extracting, and checking for updates to the Steam Linux Runtime[cite: 2].
      * `ensureProton()`: Manages the acquisition of Proton, handling both remote URLs and local user-provided paths.
      * `InstallCustomComponents()`: Copies specific DLLs (like `d3d11.dll`) into the Wine prefix for advanced override configurations.

### `internal/command`

  * **Purpose**: The bridge between YAPL and the command line. It constructs and executes external commands and contains the core logic for running games.
  * **Key Functions**:
      * `RunDirectly()`: The primary "lightweight" launch method. It executes the `wine` binary from a Proton distribution directly, bypassing the Steam Runtime.
      * `RunInContainer()`: The "heavyweight" method that launches the game inside the Steam Linux Runtime for maximum compatibility.
      * `RunWithUMU()`: Contains the specific logic for launching a game via the `umu-launcher` helper.
      * `InitializePrefix()`: Handles the creation of a new Wine prefix. It uses the `proton` script to ensure a correctly bootstrapped environment and then restructures the prefix to a standard layout.
      * `buildProtonEnv()`: A critical helper function that constructs the entire environment variable set needed by Proton. This is where `LD_LIBRARY_PATH`, `WINEPREFIX`, `STEAM_COMPAT_*`, and DLL overrides are assembled. It correctly sets Steam-specific variables if a `SteamAppID` is provided.

### `internal/archive`

  * **Purpose**: A self-contained utility for all archive-related tasks.
  * **Key Functions**:
      * `Extract()`: Takes a source URL or local path and extracts the archive to a destination. It automatically handles `gz`, `xz`, and `zst` decompression.
      * `Package()`: Creates a new compressed archive from a source directory.

### `internal/fs`

  * **Purpose**: To centralize basic, repeated filesystem operations.
  * **Key Functions**:
      * `DirExistsAndIsNotEmpty()`: A safe check to see if a directory not only exists but also contains files.
      * `MustGetAbsolutePath()`: A helper for resolving file paths.

-----

## 4\. How to Contribute

### Adding a new command:

1.  Add the command `case` to the `switch` statement in `main.go`.
2.  Create a new public method on the `App` struct in `internal/app/app.go` for the command's high-level logic.
3.  Implement the detailed logic within the relevant specialized packages (`command`, `dependency`, etc.) and call it from your new `App` method.

### Modifying launch behavior:

Changes to how games are launched or how environments are configured are made in `internal/command/command.go`, specifically within the `RunDirectly`, `RunInContainer`, and `buildProtonEnv` functions.