package upgrade

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yapl/internal/kernel"
	"yapl/internal/manifest"
	"yapl/internal/registry/api"
	"yapl/internal/registry/store"
)

func entry(name, latest string) api.CatalogEntry {
	return api.CatalogEntry{Summary: store.Summary{Name: name, Type: "game", Latest: latest}}
}

func describe(items []Item) string {
	var out []string
	for _, i := range items {
		out = append(out, i.Part+":"+i.Name+":"+i.From+"->"+i.To)
	}
	return strings.Join(out, ",")
}

func TestPlan(t *testing.T) {
	local := kernel.Versions{Kernel: "1.0.0", Modules: map[string]string{"store": "2.0.0"}}
	tests := []struct {
		name   string
		local  LocalState
		remote RemoteState
		want   string
	}{
		{
			name:   "given a newer game version on the server then the game is listed",
			local:  LocalState{Installed: []manifest.Installed{{Name: "Doom", Type: "game", Version: "1.0.0"}}, Binary: local},
			remote: RemoteState{Catalog: []api.CatalogEntry{entry("Doom", "1.1.0")}, Binary: local},
			want:   "game:Doom:1.0.0->1.1.0",
		},
		{
			name:   "given the same game version then nothing is listed",
			local:  LocalState{Installed: []manifest.Installed{{Name: "Doom", Type: "game", Version: "1.1.0"}}, Binary: local},
			remote: RemoteState{Catalog: []api.CatalogEntry{entry("Doom", "1.1.0")}, Binary: local},
			want:   "",
		},
		{
			name:   "given a game the server no longer has then it is not listed",
			local:  LocalState{Installed: []manifest.Installed{{Name: "Quake", Type: "game", Version: "1.0.0"}}, Binary: local},
			remote: RemoteState{Binary: local},
			want:   "",
		},
		{
			name:   "given a newer server binary then yapl is listed",
			local:  LocalState{Binary: local},
			remote: RemoteState{Binary: kernel.Versions{Kernel: "1.0.0", Modules: map[string]string{"store": "2.1.0"}}},
			want:   "yapl:yapl:store 2.0.0->store 2.1.0",
		},
		{
			name:   "given an outdated runner catalog then the catalog is listed",
			local:  LocalState{Binary: local, RunnerCatalog: "2025.01.0", BinaryCatalog: "2026.10.0"},
			remote: RemoteState{Binary: local},
			want:   "catalog:runner.json:2025.01.0->2026.10.0",
		},
		{
			name:   "given a runner.json without a catalog version then the catalog is listed",
			local:  LocalState{Binary: local, BinaryCatalog: "2026.10.0"},
			remote: RemoteState{Binary: local},
			want:   "catalog:runner.json:none->2026.10.0",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.local.RunnerCatalog == "" && tc.local.BinaryCatalog == "" {
				tc.local.RunnerCatalog, tc.local.BinaryCatalog = "x", "x"
			}
			if got := describe(Plan(tc.local, tc.remote)); got != tc.want {
				t.Fatalf("plan = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNewerBinary(t *testing.T) {
	base := kernel.Versions{Kernel: "1.0.0", Modules: map[string]string{"store": "2.0.0", "registry": "2.0.0"}}
	with := func(kernelVersion string, mods map[string]string) kernel.Versions {
		v := kernel.Versions{Kernel: kernelVersion, Modules: map[string]string{}}
		for k, val := range base.Modules {
			v.Modules[k] = val
		}
		for k, val := range mods {
			v.Modules[k] = val
		}
		return v
	}
	tests := []struct {
		name    string
		remote  kernel.Versions
		want    bool
		wantMsg string
	}{
		{"given identical versions then the server binary is not newer", base, false, "already"},
		{"given a newer module then the server binary is newer", with("1.0.0", map[string]string{"store": "2.1.0"}), true, "store 2.0.0 -> store 2.1.0"},
		{"given a newer kernel then the server binary is newer", with("1.1.0", nil), true, "kernel 1.0.0 -> kernel 1.1.0"},
		{"given a module the client lacks then the server binary is newer", with("1.0.0", map[string]string{"console": "1.0.0"}), true, "console new -> console 1.0.0"},
		{"given an older module then the server binary is refused as a downgrade", with("1.0.0", map[string]string{"store": "1.9.0"}), false, "older"},
		{"given one newer and one older module then the build is divergent and refused", with("1.0.0", map[string]string{"store": "2.1.0", "registry": "1.0.0"}), false, "older"},
		{"given an empty remote then nothing is known", kernel.Versions{}, false, "did not report"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, msg := NewerBinary(base, tc.remote)
			if got != tc.want || !strings.Contains(msg, tc.wantMsg) {
				t.Fatalf("NewerBinary = %v %q", got, msg)
			}
		})
	}
}

func TestInstalled(t *testing.T) {
	t.Run("given pulled games and a hand-made one then only pulled packages are reported", func(t *testing.T) {
		// Given
		dir := t.TempDir()
		orig, _ := os.Getwd()
		os.Chdir(dir)
		defer os.Chdir(orig)
		os.MkdirAll(filepath.Join("games", "Doom"), 0755)
		manifest.WriteInstalled(filepath.Join("games", "Doom"), manifest.Installed{Name: "Doom", Type: "game", Version: "1.0.0"})
		os.MkdirAll(filepath.Join("apps", "Notepad"), 0755)
		manifest.WriteInstalled(filepath.Join("apps", "Notepad"), manifest.Installed{Name: "Notepad", Type: "app", Version: "8.0.0"})
		os.MkdirAll(filepath.Join("games", "HandMade"), 0755)

		// When
		got, err := Installed()

		// Then
		if err != nil || len(got) != 2 || got[0].Name != "Doom" || got[1].Name != "Notepad" {
			t.Fatalf("installed = %+v, %v", got, err)
		}
	})
}

func TestInstalled_IncludesSymlinkedGames(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	elsewhere := filepath.Join(t.TempDir(), "Doom")
	os.MkdirAll(elsewhere, 0755)
	manifest.WriteInstalled(elsewhere, manifest.Installed{Name: "Doom", Type: "game", Version: "1.0.0"})
	os.MkdirAll("games", 0755)
	os.Symlink(elsewhere, filepath.Join("games", "Doom"))
	if got, err := Installed(); err != nil || len(got) != 1 {
		t.Fatalf("installed = %+v, %v", got, err)
	}
}
