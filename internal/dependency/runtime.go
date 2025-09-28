package dependency

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"yapl/internal/archive"
	"yapl/internal/config"
)

// EnsureRuntime checks if the Steam Linux Runtime is installed and up-to-date.
func EnsureRuntime(appCfg config.App, globalCfg config.Global) error {
	if appCfg.RuntimeVersion == "" {
		return nil // Nothing to do if no runtime is specified
	}

	runtimeInfo, ok := globalCfg.RuntimeVersions[appCfg.RuntimeVersion]
	if !ok {
		return fmt.Errorf("runtime version '%s' not defined in runner.json", appCfg.RuntimeVersion)
	}

	if runtimeInfo.URL == "" {
		return fmt.Errorf("runtime version '%s' has no URL specified in runner.json", appCfg.RuntimeVersion)
	}

	runtimeDir := filepath.Join("dependencies", "runtime", appCfg.RuntimeVersion)
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		return fmt.Errorf("could not create runtime directory: %w", err)
	}

	// Determine if an update check is needed
	updateNeeded := false
	if _, err := os.Stat(filepath.Join(runtimeDir, "version.txt")); os.IsNotExist(err) {
		updateNeeded = true // Not installed, so it needs an "update"
	} else if runtimeInfo.CheckForUpdates {
		var err error
		updateNeeded, err = runtimeNeedsUpdate(runtimeDir, runtimeInfo.URL)
		if err != nil {
			log.Printf("⚠️  Could not check for runtime update, proceeding with local version: %v", err)
		}
	}

	if !updateNeeded {
		fmt.Println("-> Steam Linux Runtime is up to date.")
		return nil
	}

	fmt.Println("-> Steam Linux Runtime needs to be installed or updated.")
	ar := &archive.Archive{Source: runtimeInfo.URL}
	if err := ar.Extract(runtimeDir); err != nil {
		fmt.Printf("❌ Runtime installation failed: %v. Cleaning up...\n", err)
		os.RemoveAll(runtimeDir)
		return err
	}

	if err := postInstallRuntimeFixup(runtimeDir, runtimeInfo.URL); err != nil {
		return fmt.Errorf("failed post-install fixup: %w", err)
	}

	fmt.Println("✅ Steam Linux Runtime setup complete.")
	return nil
}

// runtimeNeedsUpdate compares the local runtime version with the remote version.
func runtimeNeedsUpdate(runtimeDir, runtimeURL string) (bool, error) {
	localVersionFile := filepath.Join(runtimeDir, "version.txt")
	localVersion, err := os.ReadFile(localVersionFile)
	if err != nil {
		return true, fmt.Errorf("could not read local version file: %w", err)
	}

	// Correctly parse the base URL to avoid the "no Host in request URL" error.
	parsedURL, err := url.Parse(runtimeURL)
	if err != nil {
		return false, fmt.Errorf("could not parse runtime URL: %w", err)
	}
	parsedURL.Path = filepath.Dir(parsedURL.Path) + "/BUILD_ID.txt"
	buildIDURL := parsedURL.String()

	resp, err := http.Get(buildIDURL)
	if err != nil {
		return false, fmt.Errorf("could not fetch remote BUILD_ID: %w", err)
	}
	defer resp.Body.Close()

	remoteVersion, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("could not read remote BUILD_ID: %w", err)
	}

	return strings.TrimSpace(string(localVersion)) != strings.TrimSpace(string(remoteVersion)), nil
}

// postInstallRuntimeFixup performs tasks after extraction, like creating shims and version files.
func postInstallRuntimeFixup(runtimeDir, runtimeURL string) error {
	entryPointPath := filepath.Join(runtimeDir, "_v2-entry-point")
	yaplEntryPointPath := filepath.Join(runtimeDir, "yapl-entry-point")
	if _, err := os.Stat(entryPointPath); err == nil {
		if err := os.Rename(entryPointPath, yaplEntryPointPath); err != nil {
			return fmt.Errorf("failed to rename entry point: %w", err)
		}
	}

	shimPath := filepath.Join(runtimeDir, "yapl-shim")
	if err := createRuntimeShim(shimPath); err != nil {
		return fmt.Errorf("failed to create shim: %w", err)
	}

	// Correctly parse the URL to fetch the build ID.
	parsedURL, err := url.Parse(runtimeURL)
	if err != nil {
		return fmt.Errorf("could not parse runtime URL for build ID: %w", err)
	}
	parsedURL.Path = filepath.Dir(parsedURL.Path) + "/BUILD_ID.txt"
	buildIDURL := parsedURL.String()

	resp, err := http.Get(buildIDURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	remoteVersion, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(runtimeDir, "version.txt"), remoteVersion, 0644)
}

// createRuntimeShim creates a simple shell script needed by the runtime.
func createRuntimeShim(filePath string) error {
	scriptContent := `#!/bin/sh
if [ "${XDG_CURRENT_DESKTOP}" = "gamescope" ] || [ "${XDG_SESSION_DESKTOP}" = "gamescope" ]; then
  if [ "${STEAM_MULTIPLE_XWAYLANDS}" = "1" ]; then
    if [ -z "${DISPLAY}" ]; then
      export DISPLAY=":1"
    fi
  fi
fi
exec "$@"
`
	return os.WriteFile(filePath, []byte(scriptContent), 0755)
}
