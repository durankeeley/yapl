# YAPL - Developer Documentation

This document provides an in-depth overview of the YAPL codebase, architecture, and development workflow. It is intended for anyone looking to understand, modify, or contribute to the project.


## 1. Project Setup

To set up a local development environment, follow these steps:

### Clone the Repository
```bash
git clone <repository_url>
cd runner
````

### Install Dependencies

The project uses Go Modules. Dependencies will be fetched automatically on the first build. To fetch them manually:

```bash
go mod tidy
```

### Build the Binary

The main application entrypoint is located in `/cmd/yapl`. To build the executable, run the following command from the project root:

```bash
go build -o yapl ./cmd/yapl
```

This will create a `yapl` executable in the project's root directory.

-----

## 2\. Architectural Overview

The code is divided into several private packages within the `/internal` directory.

The general flow of the application is as follows:

1.  **`cmd/yapl/main.go`**: The application entrypoint. It is responsible only for parsing command-line flags and dispatching commands.
2.  **`internal/app/app.go`**: The core orchestrator. The `main` function creates an `App` instance, which holds the application's state and configuration. High-level commands like `app.Run()` or `app.Setup()` are executed from here.
3.  **Specialized Packages**: The `App` struct delegates tasks to specialized packages:
      * `internal/config`: Handles loading and saving `runner.json` and `game.json`.
      * `internal/dependency`: Manages the logic for downloading and verifying Proton and other tools.
      * `internal/command`: Responsible for all shell interactions, such as running `exec.Command`, building environment variables, and initializing Wine prefixes.
      * `internal/archive`: A utility package for creating and extracting various `.tar` archive formats.
      * `internal/fs`: Contains simple, reusable filesystem helper functions.

This layered architecture ensures that each part of the application has a single, well-defined responsibility.

-----

## 3\. Package & Function Deep Dive

### `internal/config`

  * **Purpose**: To define all configuration data structures and manage their serialization to/from JSON files.
  * **Key Functions**:
      * `LoadOrCreateGlobal()`: Reads `runner.json`. If it doesn't exist, it creates a default template.
      * `LoadOrCreateApp()`: Reads the local `game.json` or `app.json`. If it doesn't exist, it creates one.

### `internal/dependency`

  * **Purpose**: To ensure all required tools (Proton, DXVK, etc.) are downloaded and available.
  * **Key Functions**:
      * `EnsureAll()`: The main entrypoint for this package. It checks all dependencies listed in the app's config.
      * `ensureProton()`: Specifically handles the logic for downloading, extracting, and verifying a Proton build.

### `internal/command`

  * **Purpose**: The bridge between YAPL and the command line. It constructs and executes external commands. This is where the core logic for running games resides.
  * **Key Functions**:
      * `RunDirectly()`: The primary function for launching a game. It contains the crucial logic for handling both Steam and non-Steam games.
      * `buildProtonEnv()`: A critical helper function that constructs the entire environment variable set needed by Proton. This is where the distinction between a Steam and non-Steam launch context is made. If a `SteamAppID` is provided in the config, it sets Steam-specific variables; otherwise, it creates a generic environment.
      * `InitializePrefix()`: Handles the creation of a new Wine prefix using either `proton` or `wineboot`.
      * `RunWithUMU()`: Contains the specific logic for launching a game via `umu-launcher`.

### `internal/archive`

  * **Purpose**: A self-contained utility for all archive-related tasks.
  * **Key Functions**:
      * `Extract()`: Takes a source URL or local path and extracts the archive to a destination. It automatically handles `gz`, `xz`, and `zst` decompression.
      * `Package()`: Creates a new compressed archive from a source directory.

### `internal/fs`

  * **Purpose**: To centralize basic, repeated filesystem operations.
  * **Key Functions**:
      * `DirExistsAndIsNotEmpty()`: A safe check to see if a directory not only exists but also contains files.
      * `CopyFile()`: A simple file copy utility.

-----

## 4\. How to Contribute

### Adding a new command:

1.  Add the command `case` to the `switch` statement in `cmd/yapl/main.go`.
2.  Create a new public method on the `App` struct in `internal/app/app.go` for the command's high-level logic.
3.  Implement the detailed logic within the relevant specialized packages (`command`, `dependency`, etc.) and call it from your new `App` method.

### Modifying launch behavior:

Changes to how games are launched or how environments are configured are made in `internal/command/command.go`, specifically within the `RunDirectly` and `buildProtonEnv` functions.
