// Package catalog is the pinned, known-good set of components a fresh YAPL
// library starts with, so `setup` works without editing any JSON.
package catalog

import (
	"yapl/internal/config"
	"yapl/internal/kernel"
	"yapl/internal/paths"
)

// Version is calendar-versioned (YYYY.MM.N) and bumped whenever a pin changes.
const Version = "2026.10.0"

const (
	ProtonVersion  = "GE-Proton11-7"
	RuntimeVersion = "sniper"
	DXVKVersion    = "3.1.1"
	VKD3DVersion   = "3.0.1"

	protonURL  = "https://github.com/GloriousEggroll/proton-ge-custom/releases/download/GE-Proton11-7/GE-Proton11-7-x86_64.tar.gz"
	runtimeURL = "https://repo.steampowered.com/steamrt-images-sniper/snapshots/latest-container-runtime-public-beta/SteamLinuxRuntime_sniper.tar.xz"
	dxvkURL    = "https://github.com/doitsujin/dxvk/releases/download/v3.1.1/dxvk-3.1.1.tar.gz"
	vkd3dURL   = "https://github.com/HansKristian-Work/vkd3d-proton/releases/download/v3.0.1/vkd3d-proton-3.0.1.tar.zst"
)

// Default returns a fresh copy of the pinned runner configuration.
func Default() config.Global {
	return config.Global{
		ProtonVersions: map[string]config.VersionInfo{
			ProtonVersion: {URL: protonURL},
		},
		RuntimeVersions: map[string]config.VersionInfo{
			RuntimeVersion: {URL: runtimeURL, CheckForUpdates: true},
		},
		DependencyVersions: map[string]map[string]config.VersionInfo{
			paths.KindDXVK:  {DXVKVersion: {URL: dxvkURL}},
			paths.KindVKD3D: {VKD3DVersion: {URL: vkd3dURL}},
		},
		Defaults: config.Defaults{
			Proton:  ProtonVersion,
			Runtime: RuntimeVersion,
			DXVK:    DXVKVersion,
			VKD3D:   VKD3DVersion,
		},
		CatalogVersion: Version,
	}
}

// Module reports the catalog's version alongside the code modules, so
// `yapl version` and the registry can tell when pins are out of date.
type Module struct{}

func (Module) Name() string               { return "catalog" }
func (Module) Version() string            { return Version }
func (Module) Commands() []kernel.Command { return nil }
