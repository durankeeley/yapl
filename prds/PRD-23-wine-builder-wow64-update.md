# PRD-23 - Update Wine Builder to Use WoW64 Single-Build Approach

**Status:** `[ ] Pending`
**Priority:** High
**Size:** M
**Sprint:** Backlog
**Tags:** `wine-builder`, `wow64`, `docker`, `refactor`
**Created:** 2026-04-27

---

## 1. Problem & Context

`wine-builder/build-wine.sh` uses the old dual-build approach: a 64-bit build (`--enable-win64`) plus a separate 32-bit build (`--with-wine64=../wine64-build`). Since Wine 11.0, WoW64 mode allows a single 64-bit Wine process to run 32-bit and 16-bit Windows applications with no 32-bit system libraries required. The dual-build approach is now obsolete, produces larger binaries, is slower to compile, and requires maintaining i386 libraries in the container.

Additionally:
- The output is not packaged into a tarball that YAPL can use as a `proton_version` with a local `path` entry.
- There is no `batect.yml` task to invoke the wine builder.
- The build script builds both 32-bit and 64-bit in the same container run with no option to skip the 32-bit step.

* **Current State:** Dual-build (64-bit + 32-bit Wine), no output packaging, no batect task.
* **Impact/Need:** The builder produces a result YAPL cannot directly use, and the build process is slower and more complex than necessary for Wine 11+.

## 2. Proposed Solution

* Update `build-wine.sh` to use the WoW64 single-build approach: configure with `--enable-win64 --enable-wow64`, build only the 64-bit tree.
* After `make install`, package the output into a `wine-<version>.tar.xz` tarball placed in `/output`.
* Add a `wine-builder` task to `batect.yml` that builds the Docker image and runs it with a version argument.
* Update `Dockerfile` to remove i386 library packages (no longer needed for WoW64 build).

## 3. Scope & Design Details

### build-wine.sh WoW64 changes
* **Behavior/Rule:** Replace the dual-build section with a single configure+build:
  ```bash
  mkdir -p wine-build
  cd wine-build
  ../wine/configure --enable-win64 --enable-wow64 --prefix=/wine-install
  make -j"$(nproc)"
  make install
  cd ..
  ```
* **Specifics:** Remove `wine32-build` directory creation and the second configure/make/install.

### Output packaging
* **Behavior/Rule:** After `make install`, create `${OUTPUT_DIR}/wine-${WINE_VERSION}.tar.xz` containing the installed Wine files.
* **Specifics:**
  ```bash
  cd /wine-install
  tar -cJf "${OUTPUT_DIR}/wine-${WINE_VERSION}.tar.xz" .
  ```
  This produces a tarball YAPL can extract and reference via `path` in `runner.json`.

### Dockerfile i386 cleanup
* **Behavior/Rule:** Remove all `:i386` package installations from the Dockerfile. The WoW64 build only needs 64-bit libraries.
* **Specifics:** Remove the `dpkg --add-architecture i386` line and all `libX:i386` package entries.

### batect.yml wine-builder task
* **Behavior/Rule:** Add a `wine-builder` container and a `build-wine` task to `batect.yml`:
  ```yaml
  containers:
    wine-builder:
      build_directory: wine-builder
      volumes:
        - local: wine-builder/output
          container: /output
  tasks:
    build-wine:
      description: Builds a Wine version using the wine-builder Docker container
      run:
        container: wine-builder
        command: "${WINE_VERSION:-11.7}"
  ```

## 4. Execution & Milestones

- [ ] Update `wine-builder/build-wine.sh` to single WoW64 build
- [ ] Add tarball packaging step to `build-wine.sh`
- [ ] Update `wine-builder/Dockerfile` to remove i386 packages
- [ ] Add `wine-builder` container and `build-wine` task to `batect.yml`
- [ ] Build wine 11.7 with the updated builder and verify the output tarball
- [ ] Confirm YAPL can use the tarball via `path` in `runner.json`
- [ ] Mark Complete, move to `prds/done/`

## 5. Technical Notes

### Key Files & Systems Targeted
* `wine-builder/build-wine.sh` — WoW64 build, output packaging
* `wine-builder/Dockerfile` — remove i386 packages
* `batect.yml` — add wine-builder task

### Risks & Considerations
* **WoW64 flag availability:** `--enable-wow64` was stabilised in Wine 8.x. All versions ≥ 9.0 support it. No concern for Wine 11.7.
* **Build time:** Even the single 64-bit build takes 20-60 minutes depending on CPU. The Docker container uses `nproc` to maximise parallelism.
* **`ccache` in container:** Already configured with `ENV CC="ccache gcc"` in the Dockerfile. Rebuild times for incremental builds are much faster.
* **YAPL integration:** After building, add a `path` entry to `runner.json`:
  ```json
  "proton_versions": {
    "wine-11.7": { "path": "/path/to/wine-builder/output" }
  }
  ```
