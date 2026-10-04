package app

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yapl/internal/config"
	"yapl/internal/manifest"
)

func packageFixture(t *testing.T, cfg config.App) *App {
	t.Helper()
	dir := t.TempDir()
	chdir(t, dir)
	writeGameConfig(t, dir, "Doom", cfg)
	os.WriteFile(filepath.Join("games", "Doom", "doom.wad"), []byte("wad"), 0644)
	return New("games", "Doom", false, false, false, config.Global{}, cfg)
}

var containerConfig = config.App{ProtonVersion: "GE-Proton11-7", RuntimeVersion: "sniper", LaunchMethod: "container", Executable: "drive_c/doom.exe", SteamAppID: "2280"}

func fileSHA(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestPackage_WritesManifestSidecar(t *testing.T) {
	t.Run("given a game without metadata when packaged then metadata is created and the sidecar describes the archive", func(t *testing.T) {
		// Given
		a := packageFixture(t, containerConfig)

		// When
		m, err := a.Package(PackageOptions{Format: "gz", OutputDir: "dist", Yes: true, CreatedBy: "yapl test"})

		// Then
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join("games", "Doom", manifest.MetadataFile)); err != nil {
			t.Fatal("metadata.json was not created in the game directory")
		}
		onDisk, err := manifest.Read(manifest.SidecarPath("dist", "Doom"))
		if err != nil {
			t.Fatalf("sidecar: %v", err)
		}
		if onDisk.Archive.SHA256 != fileSHA(t, filepath.Join("dist", "Doom.tar.gz")) {
			t.Fatal("sidecar checksum does not match the archive")
		}
		if onDisk.Metadata.Version != manifest.DefaultVersion || onDisk.Type != "game" || onDisk.CreatedBy != "yapl test" || onDisk.Archive.File != "Doom.tar.gz" {
			t.Fatalf("sidecar = %+v", onDisk)
		}
		if len(onDisk.Components) != 2 || onDisk.Components[0].String() != "proton/GE-Proton11-7" || onDisk.Components[1].String() != "runtime/sniper" {
			t.Fatalf("components = %v", onDisk.Components)
		}
		if m.Archive.SHA256 != onDisk.Archive.SHA256 {
			t.Fatal("returned manifest differs from sidecar")
		}
	})

	t.Run("given a version flag when packaged then metadata.json and the manifest carry it", func(t *testing.T) {
		// Given
		a := packageFixture(t, containerConfig)

		// When
		m, err := a.Package(PackageOptions{Format: "gz", OutputDir: "dist", Version: "2.1.0", Yes: true})

		// Then
		if err != nil {
			t.Fatal(err)
		}
		saved, _, _ := manifest.LoadMetadata(filepath.Join("games", "Doom"), "Doom")
		if m.Metadata.Version != "2.1.0" || saved.Version != "2.1.0" {
			t.Fatalf("manifest %s, metadata %s", m.Metadata.Version, saved.Version)
		}
	})

	t.Run("given an invalid version when packaged then nothing is written", func(t *testing.T) {
		// Given
		a := packageFixture(t, containerConfig)

		// When
		_, err := a.Package(PackageOptions{Format: "gz", OutputDir: "dist", Version: "banana", Yes: true})

		// Then
		if err == nil || !strings.Contains(err.Error(), "banana") {
			t.Fatalf("error = %v", err)
		}
		if _, err := os.Stat("dist"); !os.IsNotExist(err) {
			t.Fatal("output written despite an invalid version")
		}
		if _, err := os.Stat(filepath.Join("games", "Doom", manifest.MetadataFile)); !os.IsNotExist(err) {
			t.Fatal("metadata.json written despite an invalid version")
		}
	})

	t.Run("given cover art in the game directory when packaged then it is copied next to the archive", func(t *testing.T) {
		// Given
		a := packageFixture(t, containerConfig)
		os.WriteFile(filepath.Join("games", "Doom", "cover.png"), []byte("png"), 0644)

		// When
		if _, err := a.Package(PackageOptions{Format: "gz", OutputDir: "dist", Yes: true}); err != nil {
			t.Fatal(err)
		}

		// Then
		if data, err := os.ReadFile(manifest.CoverSidecarPath("dist", "Doom", ".png")); err != nil || string(data) != "png" {
			t.Fatalf("cover sidecar missing: %v", err)
		}
	})

	t.Run("given bundled dependencies when packaged then the manifest says so", func(t *testing.T) {
		// Given
		a := packageFixture(t, config.App{ProtonVersion: "ge", LaunchMethod: "direct", Executable: "x.exe"})
		os.MkdirAll(filepath.Join("proton", "ge"), 0755)
		os.WriteFile(filepath.Join("proton", "ge", "proton"), []byte("#!/bin/sh"), 0755)

		// When
		m, err := a.Package(PackageOptions{Format: "gz", OutputDir: "dist", BundleDeps: true, Yes: true})

		// Then
		if err != nil || !m.Bundled {
			t.Fatalf("bundled = %v, err = %v", m.Bundled, err)
		}
	})
}

func TestPackage_FetchCover(t *testing.T) {
	t.Run("given a steam app id and no cover when packaged with fetch-cover then the Steam cover is downloaded", func(t *testing.T) {
		// Given
		var requested string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requested = r.URL.Path
			w.Write([]byte("jpeg"))
		}))
		defer srv.Close()
		orig := steamCoverURL
		steamCoverURL = srv.URL + "/apps/%s/library_600x900.jpg"
		defer func() { steamCoverURL = orig }()
		a := packageFixture(t, containerConfig)

		// When
		if _, err := a.Package(PackageOptions{Format: "gz", OutputDir: "dist", FetchCover: true, Yes: true}); err != nil {
			t.Fatal(err)
		}

		// Then
		if requested != "/apps/2280/library_600x900.jpg" {
			t.Fatalf("requested %q", requested)
		}
		if data, err := os.ReadFile(filepath.Join("games", "Doom", "cover.jpg")); err != nil || string(data) != "jpeg" {
			t.Fatal("cover.jpg not saved in the game directory")
		}
	})

	t.Run("given the cover download fails when packaged then packaging still succeeds", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		defer srv.Close()
		orig := steamCoverURL
		steamCoverURL = srv.URL + "/%s.jpg"
		defer func() { steamCoverURL = orig }()
		a := packageFixture(t, containerConfig)
		if _, err := a.Package(PackageOptions{Format: "gz", OutputDir: "dist", FetchCover: true, Yes: true}); err != nil {
			t.Fatalf("packaging failed because of the cover: %v", err)
		}
	})
}
