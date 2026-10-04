// Package paths defines YAPL's on-disk library layout. Every path is
// relative to the library root (the working directory, see --workdir).
package paths

import "path/filepath"

// HomeEnv names the environment variable that sets the library root when
// --workdir is not given.
const HomeEnv = "YAPL_HOME"

const (
	RunnerConfig    = "runner.json"
	ClientConfig    = "client.json"
	ProtonDir       = "proton"
	DependenciesDir = "dependencies"
	GamesDir        = "games"
	AppsDir         = "apps"
	PrefixDir       = "prefix"
	GameConfig      = "game.json"
	AppConfig       = "app.json"
	// StagingDir holds in-progress downloads inside the library, on the same
	// filesystem as their destination so finishing is an atomic rename.
	StagingDir = ".staging"
	// ConsoleBrowserDir is the browser profile `yapl console` uses, kept
	// apart from the user's own.
	ConsoleBrowserDir = ".console-browser"
	// BundleDir is the directory inside a --bundle-deps archive that carries
	// Proton, dependencies and runner.json.
	BundleDir = "_bundle"
)

// Library entry kinds, as used on the command line and in package manifests.
const (
	KindGame = "game"
	KindApp  = "app"
)

// Component kinds. Each is also the directory name the component lives in.
const (
	KindProton  = "proton"
	KindRuntime = "runtime"
	KindDXVK    = "dxvk"
	KindVKD3D   = "vkd3d"
	KindUMU     = "umu-launcher"
)

func Proton(version string) string { return filepath.Join(ProtonDir, version) }

func Dependency(kind, version string) string {
	return filepath.Join(DependenciesDir, kind, version)
}

func Runtime(version string) string { return Dependency(KindRuntime, version) }

// Component resolves any component kind, including Proton, to its directory.
func Component(kind, version string) string {
	if kind == KindProton {
		return Proton(version)
	}
	return Dependency(kind, version)
}

// AppDir is the directory of a game or app; appType is GamesDir or AppsDir.
func AppDir(appType, name string) string { return filepath.Join(appType, name) }

func Prefix(appDir string) string { return filepath.Join(appDir, PrefixDir) }

// DefaultConfigName is game.json for games and app.json for apps.
func DefaultConfigName(appType string) string {
	if appType == AppsDir {
		return AppConfig
	}
	return GameConfig
}

// TypeDir maps a kind ("game"/"app") to its library directory.
func TypeDir(kind string) string {
	if kind == KindApp {
		return AppsDir
	}
	return GamesDir
}

// KindOf maps a library directory to its kind.
func KindOf(appType string) string {
	if appType == AppsDir {
		return KindApp
	}
	return KindGame
}
