package console

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yapl/internal/archive"
	"yapl/internal/client"
	"yapl/internal/manifest"
	"yapl/internal/registry/api"
	"yapl/internal/registry/store"
)

func publishGame(t *testing.T, st *store.Store, ver string) {
	t.Helper()
	src := filepath.Join(t.TempDir(), "Doom")
	os.MkdirAll(src, 0755)
	os.WriteFile(filepath.Join(src, "game.json"), []byte(readyConfig), 0644)
	var buf bytes.Buffer
	archive.Write(&buf, src, filepath.Dir(src), "gz")
	sum := sha256.Sum256(buf.Bytes())
	u, _ := st.NewUpload()
	u.WriteArchive(bytes.NewReader(buf.Bytes()))
	u.WriteCover(".png", strings.NewReader("png"))
	if err := u.CommitPackage("Doom", ver, manifest.Manifest{SchemaVersion: 1, Name: "Doom", Type: "game", Metadata: manifest.Metadata{Title: "Doom", Version: ver},
		Archive: manifest.Archive{File: "Doom.tar.gz", Format: "gz", SHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(buf.Len())}}); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteRegistry(t *testing.T) {
	t.Run("given a real registry then the adapter lists, installs, upgrades and fetches covers", func(t *testing.T) {
		// Given
		inLibrary(t)
		st := store.New(t.TempDir(), io.Discard)
		st.Refresh()
		publishGame(t, st, "1.0.0")
		srv := httptest.NewServer(api.New(api.Config{Store: st, Auth: api.AuthOpen}))
		defer srv.Close()
		reg := NewRegistry(client.New(client.Config{Server: srv.URL}))

		// When / Then
		if !reg.Reachable() || reg.Server() != srv.URL {
			t.Fatal("registry not reachable")
		}
		cat, err := reg.Catalog()
		if err != nil || len(cat) != 1 {
			t.Fatalf("catalog %+v %v", cat, err)
		}
		if err := reg.Install("Doom", nil); err != nil {
			t.Fatal(err)
		}
		publishGame(t, st, "1.1.0")
		if err := reg.Upgrade("Doom", nil); err != nil {
			t.Fatal(err)
		}
		if in, _ := manifest.ReadInstalled(filepath.Join("games", "Doom")); in.Version != "1.1.0" {
			t.Fatalf("installed %+v", in)
		}
		body, contentType, err := reg.Cover("Doom")
		if err != nil || contentType != "image/png" {
			t.Fatalf("cover %q %v", contentType, err)
		}
		body.Close()
	})

	t.Run("given an unreachable server then the adapter reports it", func(t *testing.T) {
		reg := NewRegistry(client.New(client.Config{Server: "127.0.0.1:1"}))
		if reg.Reachable() {
			t.Fatal("unreachable server reported reachable")
		}
	})
}
