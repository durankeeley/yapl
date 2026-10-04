package client_test

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yapl/internal/client"
	"yapl/internal/config"
	"yapl/internal/manifest"
	"yapl/internal/progress"
	"yapl/internal/registry/api"
	"yapl/internal/registry/store"
)

func registry(t *testing.T, allowPush bool) (*client.Client, *store.Store) {
	t.Helper()
	st := store.New(t.TempDir(), io.Discard)
	st.Refresh()
	srv := httptest.NewServer(api.New(api.Config{Store: st, Auth: "open", AllowPush: allowPush, Info: api.Info{Kernel: "1.0.0"}}))
	t.Cleanup(srv.Close)
	return client.New(client.Config{Server: srv.URL, Auth: "open"}), st
}

func TestInfo(t *testing.T) {
	t.Run("given a v1 registry when info is requested then its api version and push setting are returned", func(t *testing.T) {
		c, _ := registry(t, true)
		info, err := c.Info()
		if err != nil || info.APIVersion != api.APIVersion || !info.PushEnabled || info.Kernel != "1.0.0" {
			t.Fatalf("info = %+v, %v", info, err)
		}
	})
}

func TestPushComponent(t *testing.T) {
	t.Run("given a streamed component when pushed then the server stores it with the streamed checksum", func(t *testing.T) {
		// Given
		c, st := registry(t, true)
		cm := manifest.ComponentManifest{SchemaVersion: 1, Kind: "proton", Version: "GE-Proton11-7", VersionInfo: config.VersionInfo{BinPath: "files/bin"}}
		var updates int

		// When
		has, _ := c.HasComponent("proton", "GE-Proton11-7")
		err := c.PushComponent(cm, func(w io.Writer) error {
			_, err := io.WriteString(w, strings.Repeat("proton", 1000))
			return err
		}, func(u progress.Update) { updates++ })

		// Then
		if err != nil {
			t.Fatal(err)
		}
		hasAfter, _ := c.HasComponent("proton", "GE-Proton11-7")
		stored, err := st.ResolveComponent("proton", "GE-Proton11-7")
		sum := sha256.Sum256([]byte(strings.Repeat("proton", 1000)))
		if has || !hasAfter || err != nil || stored.Manifest.Archive.SHA256 != hex.EncodeToString(sum[:]) || stored.Manifest.VersionInfo.BinPath != "files/bin" {
			t.Fatalf("has %v→%v stored %+v err %v", has, hasAfter, stored, err)
		}
		if updates == 0 {
			t.Fatal("no progress reported")
		}
	})

	t.Run("given a source that fails when pushing then the error is returned and nothing is stored", func(t *testing.T) {
		c, st := registry(t, true)
		cm := manifest.ComponentManifest{SchemaVersion: 1, Kind: "dxvk", Version: "3.1.1"}
		err := c.PushComponent(cm, func(w io.Writer) error { return io.ErrUnexpectedEOF }, nil)
		if err == nil || len(st.Components()) != 0 {
			t.Fatalf("err %v components %+v", err, st.Components())
		}
	})
}

func TestPushPackage(t *testing.T) {
	t.Run("given an archive and cover when pushed then the version is published with its cover", func(t *testing.T) {
		// Given
		c, st := registry(t, true)
		dir := t.TempDir()
		archivePath := filepath.Join(dir, "Doom.tar.zst")
		os.WriteFile(archivePath, []byte("doom"), 0644)
		coverPath := filepath.Join(dir, "Doom.cover.jpg")
		os.WriteFile(coverPath, []byte("jpg"), 0644)
		sum := sha256.Sum256([]byte("doom"))
		m := manifest.Manifest{SchemaVersion: 1, Name: "Doom", Type: "game", Metadata: manifest.Metadata{Title: "Doom", Version: "1.0.0"},
			Archive: manifest.Archive{File: "Doom.tar.zst", Format: "zst", SHA256: hex.EncodeToString(sum[:]), SizeBytes: 4}}

		// When
		err := c.PushPackage(m, archivePath, coverPath, nil)

		// Then
		v, resolveErr := st.Resolve("Doom", "1.0.0")
		if err != nil || resolveErr != nil || v.CoverPath == "" {
			t.Fatalf("push %v resolve %v version %+v", err, resolveErr, v)
		}
	})

	t.Run("given push is disabled when pushing then the server's explanation is returned", func(t *testing.T) {
		c, _ := registry(t, false)
		dir := t.TempDir()
		archivePath := filepath.Join(dir, "Doom.tar.zst")
		os.WriteFile(archivePath, []byte("doom"), 0644)
		err := c.PushPackage(manifest.Manifest{Name: "Doom", Metadata: manifest.Metadata{Version: "1.0.0"}}, archivePath, "", nil)
		if err == nil || !strings.Contains(err.Error(), "push is disabled") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("given a missing archive file when pushing then it fails before contacting the server", func(t *testing.T) {
		c, _ := registry(t, true)
		if err := c.PushPackage(manifest.Manifest{Name: "Doom", Metadata: manifest.Metadata{Version: "1.0.0"}}, "/nonexistent.tar.zst", "", nil); err == nil {
			t.Fatal("expected an error")
		}
	})
}
