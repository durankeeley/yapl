package client_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"

	"yapl/internal/client"
	"yapl/internal/manifest"
	"yapl/internal/registry/store"
)

func seed(t *testing.T, st *store.Store, name, ver, content string) {
	t.Helper()
	u, err := st.NewUpload()
	if err != nil {
		t.Fatal(err)
	}
	u.WriteArchive(strings.NewReader(content))
	sum := sha256.Sum256([]byte(content))
	m := manifest.Manifest{SchemaVersion: 1, Name: name, Type: "game", Metadata: manifest.Metadata{Title: name + " title", Version: ver},
		Archive: manifest.Archive{File: name + ".tar.gz", Format: "gz", SHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(len(content))}}
	if err := u.CommitPackage(name, ver, m); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogAndManifest(t *testing.T) {
	t.Run("given published packages when the catalog is fetched then titles and latest versions are returned", func(t *testing.T) {
		c, st := registry(t, true)
		seed(t, st, "Doom", "1.0.0", "a")
		seed(t, st, "Doom", "1.1.0", "b")
		cat, err := c.Catalog()
		if err != nil || len(cat) != 1 || cat[0].Latest != "1.1.0" || cat[0].Title != "Doom title" {
			t.Fatalf("catalog = %+v, %v", cat, err)
		}
	})

	t.Run("given a tag when the manifest is fetched then that version is returned", func(t *testing.T) {
		c, st := registry(t, true)
		seed(t, st, "Need for Speed", "2.0.0", "a")
		m, err := c.Manifest("Need for Speed", "latest")
		if err != nil || m.Metadata.Version != "2.0.0" {
			t.Fatalf("manifest = %+v, %v", m, err)
		}
	})

	t.Run("given an unknown package when the manifest is fetched then a not-found error is returned", func(t *testing.T) {
		c, _ := registry(t, true)
		_, err := c.Manifest("Quake", "latest")
		if !errors.Is(err, client.ErrNotFound) || !strings.Contains(err.Error(), "404") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestOpenArchive(t *testing.T) {
	t.Run("given a published version when its archive is opened then the bytes and size are returned", func(t *testing.T) {
		c, st := registry(t, true)
		seed(t, st, "Doom", "1.0.0", "doom-bytes")
		body, size, err := c.OpenArchive("Doom", "1.0.0")
		if err != nil {
			t.Fatal(err)
		}
		defer body.Close()
		data, _ := io.ReadAll(body)
		if string(data) != "doom-bytes" || size != int64(len("doom-bytes")) {
			t.Fatalf("data %q size %d", data, size)
		}
	})

	t.Run("given a missing version when its archive is opened then an error is returned", func(t *testing.T) {
		c, _ := registry(t, true)
		if _, _, err := c.OpenArchive("Doom", "9.9.9"); err == nil {
			t.Fatal("expected an error")
		}
	})
}

func TestComponentFetch(t *testing.T) {
	t.Run("given a pushed component when fetched then its manifest, archive and url are available", func(t *testing.T) {
		// Given
		c, _ := registry(t, true)
		cm := manifest.ComponentManifest{SchemaVersion: 1, Kind: "dxvk", Version: "3.1.1"}
		if err := c.PushComponent(cm, func(w io.Writer) error { _, err := io.WriteString(w, "dxvk"); return err }, nil); err != nil {
			t.Fatal(err)
		}

		// When
		got, err := c.ComponentManifest("dxvk", "3.1.1")
		body, size, openErr := c.OpenComponentArchive("dxvk", "3.1.1")

		// Then
		if err != nil || openErr != nil || got.Archive.SizeBytes != 4 || size != 4 {
			t.Fatalf("manifest %+v err %v open err %v size %d", got, err, openErr, size)
		}
		var buf bytes.Buffer
		io.Copy(&buf, body)
		body.Close()
		if buf.String() != "dxvk" {
			t.Fatalf("archive = %q", buf.String())
		}
		if url := c.ComponentURL("dxvk", "3.1.1"); !strings.HasSuffix(url, "/v1/components/dxvk/3.1.1/archive") || !strings.HasPrefix(url, "http://") {
			t.Fatalf("url = %q", url)
		}
	})
}

func TestServerAddress(t *testing.T) {
	t.Run("given a bare host and port then the client normalises it to a URL", func(t *testing.T) {
		c := client.New(client.Config{Server: "10.0.0.5:8471/"})
		if c.Server() != "http://10.0.0.5:8471" {
			t.Fatalf("server = %q", c.Server())
		}
	})
}
