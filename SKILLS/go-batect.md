# Skill: go-batect — Environment as Code

## What This Skill Is For

Use this skill when working in a repository that uses (or should use) `go-batect` to define development and CI tasks. It teaches you to read, write, and reason about `batect.yml` / `config.yml` files, and explains the environment-as-code philosophy behind them.

---

## Environment as Code — The Core Idea

Environment as code means the tools, runtimes, and commands needed to build, test, and run a project are committed to the repository — not installed on the host machine and not assumed to exist. Every task runs in a Docker container defined in the config file. Developers and CI pipelines run the same containers with the same flags, so "it works on my machine" is no longer a failure mode.

Key benefits:
- **Reproducibility**: the container image is the environment. Pinning the image tag pins the toolchain version.
- **Zero host dependencies**: contributors only need Docker. No language runtimes, no global CLIs, no version managers.
- **Self-documenting**: reading `batect.yml` tells you exactly what runs, in what container, with what mounts.
- **CI parity**: CI runs the same `go-batect <task>` command a developer runs locally. No separate CI-specific install scripts.
- **Onboarding speed**: `git clone` + `go-batect build` is a complete onboarding path.

---

## Tool Overview

`go-batect` is a lightweight Go reimplementation of [Batect](https://batect.dev). It reads a YAML config file and runs named tasks inside Docker containers or Docker Compose environments.

### Binary / invocation

```bash
# Run a task
./go-batect <task-name>

# Use a custom config file (defaults: config.yml then batect.yml)
./go-batect --file path/to/config.yml <task-name>
./go-batect -f path/to/config.yml <task-name>

# List all available tasks
./go-batect --list
```

Config file discovery order (when `--file` is not given):
1. `config.yml`
2. `batect.yml`

---

## Config File Reference

A config file has two top-level keys: `containers` and `tasks`.

### `containers`

Each container is a named Docker environment.

```yaml
containers:
  <name>:
    image: <docker-image>          # use a pre-built image
    build: <path-to-dockerfile>    # OR build from a Dockerfile directory
    legacy_build: false            # force docker build instead of buildx (default: false)
    volumes:
      - local: <host-path>         # relative paths are resolved to absolute
        container: <container-path>
    working_directory: <path>      # working directory inside the container
    healthcheck:
      command: "<shell command>"   # e.g. "curl -f http://localhost:8080 || exit 1"
      interval: 5s
      timeout: 2s
      retries: 3
      start_period: 10s
```

**Build behaviour:**
- `build` takes a path to a directory containing a `Dockerfile`.
- By default `docker buildx build --load` is used. If buildx fails, it automatically falls back to `docker build`.
- Set `legacy_build: true` to skip buildx and always use `docker build`.
- The built image is tagged `go-batect_<container-name>`.

**Healthcheck:**
All fields are optional. When a healthcheck is defined on a container used by a task, `go-batect` waits (up to 5 minutes, polling every 2 seconds) for the container to report `healthy` before running the command.

---

### `tasks`

Each task is a named unit of work.

```yaml
tasks:
  <name>:
    description: "Human-readable description"
    prerequisites:           # run these tasks first, in order
      - <other-task-name>
    shell: false             # wrap command in a shell (default: false)
    shell_executable: sh     # which shell to use (default: sh)
    run:
      container: <container-name>
      command: <command>
    # Docker Compose mode (mutually exclusive with plain docker run):
    docker_compose: false
    docker_compose_file: docker-compose.yml
    docker_compose_down: false   # run `docker compose down` after task completes
```

#### `prerequisites`

Tasks listed under `prerequisites` run before the current task, in the order listed. Use this to chain setup steps.

```yaml
tasks:
  lint:
    description: "Lint the code"
    run:
      container: node
      command: npm run lint

  test:
    description: "Run tests (after lint)"
    prerequisites:
      - lint
    run:
      container: node
      command: npm test

  ci:
    description: "Full CI pipeline"
    prerequisites:
      - lint
      - test
```

#### `shell` and `shell_executable`

When `shell: true`, the command is passed to a shell with `-c`. This lets you use pipes, redirects, environment variables, `&&` chains, and multi-line scripts.

```yaml
tasks:
  build-all:
    description: "Build for multiple platforms"
    shell: true
    shell_executable: sh          # optional, defaults to sh
    run:
      container: golang
      command: |
        GOOS=linux   GOARCH=amd64 go build -o bin/app_linux_amd64
        GOOS=linux   GOARCH=arm64 go build -o bin/app_linux_arm64
        GOOS=windows GOARCH=amd64 go build -o bin/app_windows_amd64.exe
```

Without `shell: true`, the command is split on whitespace and passed directly to `docker run` / `docker compose exec` — no shell interpolation.

#### Docker Compose mode

When `docker_compose: true`, `go-batect` runs `docker compose -f <file> up -d`, then uses `docker compose exec` to run the command in the named service container.

```yaml
tasks:
  integration-test:
    description: "Run integration tests against a live DB"
    docker_compose: true
    docker_compose_file: docker-compose.test.yml
    docker_compose_down: true      # tears down compose stack when done
    shell: true
    run:
      container: app
      command: npm run test:integration
```

`docker_compose_down: true` defers `docker compose down` so the stack is cleaned up even if the task fails.

If the container referenced by `run.container` has a `healthcheck` defined in the `containers` section, `go-batect` waits for that service to be healthy before executing the command.

---

## Complete Example

```yaml
containers:
  node:
    image: node:20-alpine
    volumes:
      - local: .
        container: /app
    working_directory: /app

  postgres:
    image: postgres:16-alpine
    healthcheck:
      command: "pg_isready -U postgres || exit 1"
      interval: 2s
      timeout: 1s
      retries: 10
      start_period: 5s

tasks:
  install:
    description: "Install npm dependencies"
    run:
      container: node
      command: npm ci

  lint:
    description: "Lint source code"
    prerequisites:
      - install
    run:
      container: node
      command: npm run lint

  test:
    description: "Run unit tests"
    prerequisites:
      - install
    run:
      container: node
      command: npm test

  test-integration:
    description: "Run integration tests with a real database"
    docker_compose: true
    docker_compose_file: docker-compose.yml
    docker_compose_down: true
    shell: true
    run:
      container: app
      command: npm run test:integration

  ci:
    description: "Full CI: lint → test"
    prerequisites:
      - lint
      - test
```

---

## When to Suggest go-batect

Suggest adding or using go-batect when:
- A project's README has a long "Prerequisites" section listing tools to install.
- The CI config duplicates install steps that also appear in developer docs.
- Tests require a database or external service — `docker_compose: true` + `docker_compose_down: true` is a clean one-command integration test setup.
- A project cross-compiles for multiple platforms — shell tasks in a pinned Go container replace fragile host toolchain assumptions.
- A new contributor asks "how do I set this up?" and the answer involves more than `git clone`.

---

## Anti-Patterns to Avoid

- **Don't use `latest` image tags in production configs.** Pin to a digest or version tag for reproducibility.
- **Don't mount the entire host filesystem.** Mount only what the task needs (e.g. the project root).
- **Don't skip `docker_compose_down: true` for ephemeral test environments.** Leaked compose stacks consume resources and cause port conflicts.
- **Don't use `shell: true` when a plain command works.** Shell mode hides argument quoting issues; use it only when you need shell features.
- **Don't put secrets in `batect.yml`.** Pass them as environment variables from the host or a secrets manager; the config file is committed to source control.
