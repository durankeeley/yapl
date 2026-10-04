package paths

import (
	"path/filepath"
	"testing"
)

func TestComponentPaths(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"given a proton version then it lives under proton/", Proton("GE-Proton11-7"), filepath.Join("proton", "GE-Proton11-7")},
		{"given a dependency then it lives under dependencies/<kind>/", Dependency(KindDXVK, "3.1.1"), filepath.Join("dependencies", "dxvk", "3.1.1")},
		{"given a runtime then it is a dependency of kind runtime", Runtime("sniper"), filepath.Join("dependencies", "runtime", "sniper")},
		{"given a component of kind proton then it resolves to the proton dir", Component(KindProton, "ge"), filepath.Join("proton", "ge")},
		{"given a component of another kind then it resolves under dependencies", Component(KindVKD3D, "3.0.1"), filepath.Join("dependencies", "vkd3d", "3.0.1")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}

func TestLibraryPaths(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"given a game then its directory is games/<name>", AppDir(GamesDir, "Doom"), filepath.Join("games", "Doom")},
		{"given an app directory then its prefix is <dir>/prefix", Prefix(filepath.Join("apps", "Notepad")), filepath.Join("apps", "Notepad", "prefix")},
		{"given the games type then the default config is game.json", DefaultConfigName(GamesDir), "game.json"},
		{"given the apps type then the default config is app.json", DefaultConfigName(AppsDir), "app.json"},
		{"given the game kind then its type directory is games", TypeDir(KindGame), "games"},
		{"given the app kind then its type directory is apps", TypeDir(KindApp), "apps"},
		{"given the games directory then its kind is game", KindOf(GamesDir), "game"},
		{"given the apps directory then its kind is app", KindOf(AppsDir), "app"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}
