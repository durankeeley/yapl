package dependency

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"yapl/internal/archive"
	"yapl/internal/config"
	"yapl/internal/fs"
	"yapl/internal/paths"
)

// EnsureAll checks and acquires all configured dependencies.
func EnsureAll(appCfg config.App, forceUpgrade bool, globalCfg config.Global) error {
	fmt.Println("-> Checking dependencies...")
	if err := ensureProton(appCfg, forceUpgrade, globalCfg); err != nil {
		return err
	}
	if appCfg.LaunchMethod == "umu" && !appCfg.UMUOptions.UseSystemBinary {
		if err := ensure(paths.KindUMU, appCfg.UMUOptions.Version, globalCfg); err != nil {
			return err
		}
	}
	if err := ensure(paths.KindDXVK, appCfg.Dependencies.DXVKVersion, globalCfg); err != nil {
		return err
	}
	if err := ensure(paths.KindVKD3D, appCfg.Dependencies.VKD3DVersion, globalCfg); err != nil {
		return err
	}
	return nil
}

func ensureProton(appCfg config.App, forceUpgrade bool, globalCfg config.Global) error {
	// "system" means use whatever wine is in PATH — nothing to download or validate.
	if appCfg.ProtonVersion == "system" {
		return nil
	}

	vinfo, ok := globalCfg.ProtonVersions[appCfg.ProtonVersion]
	if !ok {
		return fmt.Errorf("proton version '%s' not defined in runner.json", appCfg.ProtonVersion)
	}

	protonPath := paths.Proton(appCfg.ProtonVersion)
	if vinfo.Path != "" {
		if _, err := os.Stat(vinfo.Path); os.IsNotExist(err) {
			return fmt.Errorf("custom proton path does not exist: %s", vinfo.Path)
		}
		fmt.Println("-> Using local Proton version.")
		return nil
	}

	if vinfo.URL == "" {
		return fmt.Errorf("proton version '%s' has no URL in runner.json", appCfg.ProtonVersion)
	}
	if !fs.DirExistsAndIsNotEmpty(protonPath) || forceUpgrade {
		fmt.Printf("-> Acquiring Proton '%s'...\n", appCfg.ProtonVersion)
		if forceUpgrade {
			if err := os.RemoveAll(protonPath); err != nil {
				return fmt.Errorf("failed to remove existing proton path: %w", err)
			}
		}
		ar := &archive.Archive{Source: vinfo.URL}
		if err := ar.Extract(protonPath, true); err != nil {
			return fmt.Errorf("failed to acquire proton: %w", err)
		}
	}
	return nil
}

func ensure(name, version string, globalCfg config.Global) error {
	if version == "" {
		return nil
	}
	depPath := paths.Dependency(name, version)
	if fs.DirExistsAndIsNotEmpty(depPath) {
		return nil
	}

	vinfo, err := getInfo(name, version, globalCfg)
	if err != nil {
		return err
	}
	fmt.Printf("-> Acquiring %s '%s'...\n", name, version)
	ar := &archive.Archive{Source: vinfo.URL}
	if err := ar.Extract(depPath, true); err != nil {
		return fmt.Errorf("failed to acquire dependency '%s': %w", name, err)
	}
	return nil
}

// InstallCustomComponents copies specific DLLs to the Wine prefix for custom DXVK/VKD3D setups.
func InstallCustomComponents(prefixPath string, deps config.AppDependencies) error {
	dxvkMap := map[string][]string{
		"9":  {"d3d9.dll"},
		"10": {"d3d10.dll", "d3d10_1.dll", "d3d10core.dll", "d3d11.dll", "dxgi.dll"},
		"11": {"d3d11.dll", "dxgi.dll"},
	}
	vkd3dList := []string{"d3d12.dll", "d3d12core.dll"}

	dxvkDLLs, known := dxvkMap[deps.DXVKDirectXVersion]
	if deps.DXVKDirectXVersion != "" && !known {
		return fmt.Errorf("unrecognised dxvk_directx_version %q; valid values are 9, 10, 11", deps.DXVKDirectXVersion)
	}
	if err := install(paths.KindDXVK, deps.DXVKVersion, deps.DXVKInstallPath, prefixPath, dxvkDLLs); err != nil {
		return err
	}
	if err := install(paths.KindVKD3D, deps.VKD3DVersion, deps.VKD3DInstallPath, prefixPath, vkd3dList); err != nil {
		return err
	}
	return nil
}

func install(name, version, installPath, prefixPath string, dlls []string) error {
	if installPath == "" || version == "" || len(dlls) == 0 {
		return nil
	}
	fmt.Printf("-> Installing custom %s DLLs...\n", name)
	sourceDir := filepath.Join(paths.Dependency(name, version), "x64")
	absPrefixPath, err := fs.GetAbsolutePath(prefixPath)
	if err != nil {
		return fmt.Errorf("could not resolve prefix path: %w", err)
	}
	destDir := filepath.Join(absPrefixPath, "drive_c", installPath)
	if err := fs.MustCreateDirectory(destDir); err != nil {
		return err
	}
	for _, file := range dlls {
		srcPath := filepath.Join(sourceDir, file)
		dstPath := filepath.Join(destDir, file)
		if err := fs.CopyFile(srcPath, dstPath); err != nil {
			log.Printf("⚠️  Failed to copy %s: %v", file, err)
		}
	}
	return nil
}

// EnsureWinetricks runs winetricks to install the requested packages into the Wine prefix.
// It is a no-op when packages is empty or all are already recorded in prefix/winetricks.log.
// The winetricks binary must be in PATH.
func EnsureWinetricks(prefixPath string, packages []string) error {
	if len(packages) == 0 {
		return nil
	}
	toInstall := filterUninstalledPackages(prefixPath, packages)
	if len(toInstall) == 0 {
		return nil
	}
	winetricksPath, err := exec.LookPath("winetricks")
	if err != nil {
		return fmt.Errorf("winetricks not found in PATH; install it to use the winetricks config option")
	}
	absPrefix, err := filepath.Abs(prefixPath)
	if err != nil {
		return fmt.Errorf("could not resolve prefix path: %w", err)
	}
	args := append([]string{"--unattended"}, toInstall...)
	cmd := exec.Command(winetricksPath, args...)
	cmd.Env = append(os.Environ(), "WINEPREFIX="+absPrefix)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("winetricks failed: %w", err)
	}
	return nil
}

// filterUninstalledPackages reads prefix/winetricks.log and returns only those packages
// from the requested list that are not already recorded there.
func filterUninstalledPackages(prefixPath string, packages []string) []string {
	logPath := filepath.Join(prefixPath, "winetricks.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		return packages
	}
	installed := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		if pkg := strings.TrimSpace(line); pkg != "" {
			installed[pkg] = true
		}
	}
	var missing []string
	for _, pkg := range packages {
		if !installed[pkg] {
			missing = append(missing, pkg)
		}
	}
	return missing
}

func getInfo(name, version string, globalCfg config.Global) (config.VersionInfo, error) {
	vinfoMap, ok := globalCfg.DependencyVersions[name]
	if !ok {
		return config.VersionInfo{}, fmt.Errorf("dependency type '%s' not defined in runner.json", name)
	}
	vinfo, ok := vinfoMap[version]
	if !ok {
		return config.VersionInfo{}, fmt.Errorf("version '%s' for '%s' not defined in runner.json", version, name)
	}
	return vinfo, nil
}
