package dependency

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"yapl/internal/archive"
	"yapl/internal/config"
	"yapl/internal/fs"
)

// EnsureAll checks and acquires all configured dependencies.
func EnsureAll(appCfg config.App, forceUpgrade bool, globalCfg config.Global) error {
	fmt.Println("-> Checking dependencies...")
	if err := ensureProton(appCfg, forceUpgrade, globalCfg); err != nil {
		return err
	}
	if appCfg.LaunchMethod == "umu" && !appCfg.UMUOptions.UseSystemBinary {
		if err := ensure("umu-launcher", appCfg.UMUOptions.Version, globalCfg); err != nil {
			return err
		}
	}
	if err := ensure("dxvk", appCfg.Dependencies.DXVKVersion, globalCfg); err != nil {
		return err
	}
	if err := ensure("vkd3d", appCfg.Dependencies.VKD3DVersion, globalCfg); err != nil {
		return err
	}
	return nil
}

func ensureProton(appCfg config.App, forceUpgrade bool, globalCfg config.Global) error {
	vinfo, ok := globalCfg.ProtonVersions[appCfg.ProtonVersion]
	if !ok {
		return fmt.Errorf("proton version '%s' not defined in runner.json", appCfg.ProtonVersion)
	}

	protonPath := filepath.Join("proton", appCfg.ProtonVersion)
	if vinfo.Path != "" {
		if _, err := os.Stat(vinfo.Path); os.IsNotExist(err) {
			return fmt.Errorf("custom proton path does not exist: %s", vinfo.Path)
		}
		fmt.Println("-> Using local Proton version.")
	} else {
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
			if err := ar.Extract(protonPath); err != nil {
				return fmt.Errorf("failed to acquire proton: %w", err)
			}
		}
	}

	if appCfg.WineArch == "win32" {
		return patchProtonForWin32(appCfg.ProtonVersion)
	}

	return nil
}

func ensure(name, version string, globalCfg config.Global) error {
	if version == "" {
		return nil
	}
	depPath := filepath.Join("dependencies", name, version)
	if fs.DirExistsAndIsNotEmpty(depPath) {
		return nil
	}

	vinfo, err := getInfo(name, version, globalCfg)
	if err != nil {
		return err
	}
	fmt.Printf("-> Acquiring %s '%s'...\n", name, version)
	ar := &archive.Archive{Source: vinfo.URL}
	if err := ar.Extract(depPath); err != nil {
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

	if err := install("dxvk", deps.DXVKVersion, deps.DXVKInstallPath, prefixPath, dxvkMap[deps.DXVKDirectXVersion]); err != nil {
		return err
	}
	if err := install("vkd3d", deps.VKD3DVersion, deps.VKD3DInstallPath, prefixPath, vkd3dList); err != nil {
		return err
	}
	return nil
}

func install(name, version, installPath, prefixPath string, dlls []string) error {
	if installPath == "" || version == "" || len(dlls) == 0 {
		return nil
	}
	fmt.Printf("-> Installing custom %s DLLs...\n", name)
	sourceDir := filepath.Join("dependencies", name, version, "x64")
	destDir := filepath.Join(fs.MustGetAbsolutePath(prefixPath), "drive_c", installPath)
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

func patchProtonForWin32(version string) error {
	originalPath := filepath.Join("proton", version)
	patchedPath := filepath.Join("proton", version+"-win32")

	if fs.DirExistsAndIsNotEmpty(patchedPath) {
		fmt.Println("-> Found existing patched Proton for win32.")
		return nil
	}

	fmt.Printf("-> Creating patched Proton version for win32 at '%s'...\n", patchedPath)

	if err := fs.CopyDir(originalPath, patchedPath); err != nil {
		return fmt.Errorf("failed to copy proton directory for win32 patch: %w", err)
	}

	protonScriptPath := filepath.Join(patchedPath, "proton")
	scriptBytes, err := os.ReadFile(protonScriptPath)
	if err != nil {
		return fmt.Errorf("could not read proton script for patching: %w", err)
	}

	modifiedScript := strings.ReplaceAll(string(scriptBytes), "wine64", "wine")

	if err := os.WriteFile(protonScriptPath, []byte(modifiedScript), 0755); err != nil {
		return fmt.Errorf("could not write patched proton script: %w", err)
	}

	fmt.Println("✅ Proton patched for win32.")
	return nil
}
