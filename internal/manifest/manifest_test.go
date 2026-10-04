package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yapl/internal/config"
)

func TestLoadMetadata(t *testing.T) {
	t.Run("given a game without metadata.json then defaults are returned and reported as new", func(t *testing.T) {
		// When
		m, existed, err := LoadMetadata(t.TempDir(), "Doom")

		// Then
		if err != nil || existed || m.Title != "Doom" || m.Version != DefaultVersion {
			t.Fatalf("metadata = %+v existed=%v err=%v", m, existed, err)
		}
	})

	t.Run("given saved metadata then it is loaded back unchanged", func(t *testing.T) {
		// Given
		dir := t.TempDir()
		want := Metadata{Title: "DOOM (1993)", Version: "1.9.0", Description: "Rip and tear", Tags: []string{"fps"}, Players: "1-4", Preserve: []string{"base/*.cfg"}}
		if err := SaveMetadata(dir, want); err != nil {
			t.Fatal(err)
		}

		// When
		got, existed, err := LoadMetadata(dir, "Doom")

		// Then
		if err != nil || !existed || got.Title != want.Title || got.Version != want.Version || got.Players != "1-4" || got.Preserve[0] != "base/*.cfg" {
			t.Fatalf("metadata = %+v existed=%v err=%v", got, existed, err)
		}
	})

	t.Run("given metadata without a title or version then those are filled from defaults", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, MetadataFile), []byte(`{"description":"x"}`), 0644)
		m, _, err := LoadMetadata(dir, "Doom")
		if err != nil || m.Title != "Doom" || m.Version != DefaultVersion || m.Description != "x" {
			t.Fatalf("metadata = %+v err=%v", m, err)
		}
	})

	t.Run("given malformed metadata then loading fails", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, MetadataFile), []byte(`{`), 0644)
		if _, _, err := LoadMetadata(dir, "Doom"); err == nil {
			t.Fatal("expected a parse error")
		}
	})
}

func TestCoverPath(t *testing.T) {
	t.Run("given a cover.png in the game directory then it is found", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "cover.png"), []byte("png"), 0644)
		if got := (Metadata{}).CoverPath(dir); got != filepath.Join(dir, "cover.png") {
			t.Fatalf("cover = %q", got)
		}
	})
	t.Run("given an explicit cover file then it wins over the default names", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "cover.jpg"), []byte("jpg"), 0644)
		os.WriteFile(filepath.Join(dir, "box.webp"), []byte("webp"), 0644)
		if got := (Metadata{Cover: "box.webp"}).CoverPath(dir); got != filepath.Join(dir, "box.webp") {
			t.Fatalf("cover = %q", got)
		}
	})
	t.Run("given no cover then the path is empty", func(t *testing.T) {
		if got := (Metadata{}).CoverPath(t.TempDir()); got != "" {
			t.Fatalf("cover = %q", got)
		}
	})
}

func TestComponentsFor(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.App
		want string
	}{
		{"given a container game then proton and the runtime are required", config.App{ProtonVersion: "GE-Proton11-7", LaunchMethod: "container", RuntimeVersion: "sniper"}, "proton/GE-Proton11-7,runtime/sniper"},
		{"given no launch method then it is treated as container", config.App{ProtonVersion: "ge", RuntimeVersion: "sniper"}, "proton/ge,runtime/sniper"},
		{"given a direct game with dxvk and vkd3d then they are required", config.App{ProtonVersion: "ge", LaunchMethod: "direct", Dependencies: config.AppDependencies{DXVKVersion: "3.1.1", VKD3DVersion: "3.0.1"}}, "proton/ge,dxvk/3.1.1,vkd3d/3.0.1"},
		{"given system wine then no proton component is required", config.App{ProtonVersion: "system", LaunchMethod: "direct"}, ""},
		{"given umu with its own binary then umu-launcher is required", config.App{ProtonVersion: "ge", LaunchMethod: "umu", UMUOptions: config.UMUOptions{Version: "1.2.9"}}, "proton/ge,umu-launcher/1.2.9"},
		{"given umu using the system binary then umu-launcher is not required", config.App{ProtonVersion: "ge", LaunchMethod: "umu", UMUOptions: config.UMUOptions{UseSystemBinary: true, Version: "1.2.9"}}, "proton/ge"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, c := range ComponentsFor(tc.cfg) {
				got = append(got, c.String())
			}
			if strings.Join(got, ",") != tc.want {
				t.Fatalf("components = %v, want %s", got, tc.want)
			}
		})
	}
}

func validManifest() Manifest {
	return Manifest{
		SchemaVersion: SchemaVersion,
		Name:          "Doom",
		Type:          "game",
		Metadata:      Metadata{Title: "Doom", Version: "1.0.0"},
		Archive:       Archive{File: "Doom.tar.xz", Format: "xz", SHA256: strings.Repeat("a", 64), SizeBytes: 10},
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Manifest)
		wantErr string
	}{
		{"given a complete manifest then it is valid", func(*Manifest) {}, ""},
		{"given a non semantic version then it is rejected", func(m *Manifest) { m.Metadata.Version = "banana" }, "version"},
		{"given an unsafe name then it is rejected", func(m *Manifest) { m.Name = "../etc" }, "name"},
		{"given an unknown type then it is rejected", func(m *Manifest) { m.Type = "movie" }, "type"},
		{"given a newer schema then it is rejected", func(m *Manifest) { m.SchemaVersion = SchemaVersion + 1 }, "schema"},
		{"given a malformed checksum then it is rejected", func(m *Manifest) { m.Archive.SHA256 = "abc" }, "sha256"},
		{"given no checksum then it is allowed for legacy archives", func(m *Manifest) { m.Archive.SHA256 = "" }, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given
			m := validManifest()
			tc.mutate(&m)

			// When
			err := m.Validate()

			// Then
			if tc.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("error = %v, want mention of %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidName(t *testing.T) {
	for _, ok := range []string{"Doom", "Half-Life", "Need for Speed Most Wanted", "Quake 3", "C&C_Red.Alert+"} {
		if !ValidName(ok) {
			t.Errorf("given %q then it should be valid", ok)
		}
	}
	for _, bad := range []string{"", ".", "..", "../x", "a/b", `a\b`, ".hidden", "a..b/c", "x\x00"} {
		if ValidName(bad) {
			t.Errorf("given %q then it should be invalid", bad)
		}
	}
}

func TestReadWrite(t *testing.T) {
	t.Run("given a manifest when written and read then it round trips", func(t *testing.T) {
		// Given
		path := filepath.Join(t.TempDir(), "Doom.manifest.json")
		want := validManifest()
		want.Components = []Component{{Kind: "proton", Version: "GE-Proton11-7"}}

		// When
		if err := Write(path, want); err != nil {
			t.Fatal(err)
		}
		got, err := Read(path)

		// Then
		if err != nil || got.Name != "Doom" || got.Archive.SHA256 != want.Archive.SHA256 || got.Components[0].Version != "GE-Proton11-7" {
			t.Fatalf("got %+v, %v", got, err)
		}
	})

	t.Run("given an invalid manifest on disk then reading fails validation", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "m.json")
		os.WriteFile(path, []byte(`{"schema_version":1,"name":"../x","type":"game","metadata":{"version":"1.0.0"}}`), 0644)
		if _, err := Read(path); err == nil {
			t.Fatal("expected validation error")
		}
	})
}

func TestSidecarPaths(t *testing.T) {
	if got := SidecarPath("out", "Doom"); got != filepath.Join("out", "Doom.manifest.json") {
		t.Fatalf("sidecar = %q", got)
	}
	if got := CoverSidecarPath("out", "Doom", ".png"); got != filepath.Join("out", "Doom.cover.png") {
		t.Fatalf("cover sidecar = %q", got)
	}
}
