package connect

import (
	"bytes"
	"errors"
	"io"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"yapl/internal/config"
	"yapl/internal/registry/api"
	"yapl/internal/registry/store"
)

func liveServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(api.New(api.Config{Store: store.New(t.TempDir(), io.Discard), Auth: api.AuthOpen}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestConnect(t *testing.T) {
	t.Run("given an explicit server then it is used without checking anything else", func(t *testing.T) {
		c, err := Connect(Options{Server: "10.0.0.9:8471", SettingsPath: filepath.Join(t.TempDir(), "client.json")}, io.Discard)
		if err != nil || c.Server() != "http://10.0.0.9:8471" {
			t.Fatalf("client %v err %v", c, err)
		}
	})

	t.Run("given a remembered server that answers then it is used", func(t *testing.T) {
		// Given
		settings := filepath.Join(t.TempDir(), "client.json")
		url := liveServer(t)
		config.SaveClientSettings(settings, config.ClientSettings{Server: url, Auth: "open"})

		// When
		c, err := Connect(Options{SettingsPath: settings}, io.Discard)

		// Then
		if err != nil || c.Server() != url {
			t.Fatalf("client %v err %v", c, err)
		}
	})

	t.Run("given a remembered server that is down and no discovery then ErrNoServer is returned", func(t *testing.T) {
		settings := filepath.Join(t.TempDir(), "client.json")
		config.SaveClientSettings(settings, config.ClientSettings{Server: "127.0.0.1:1"})
		var out bytes.Buffer
		_, err := Connect(Options{SettingsPath: settings}, &out)
		if !errors.Is(err, ErrNoServer) || !strings.Contains(out.String(), "not answering") {
			t.Fatalf("err %v out %q", err, out.String())
		}
	})

	t.Run("given nothing remembered and no discovery then ErrNoServer is returned", func(t *testing.T) {
		if _, err := Connect(Options{SettingsPath: filepath.Join(t.TempDir(), "client.json")}, io.Discard); !errors.Is(err, ErrNoServer) {
			t.Fatalf("err %v", err)
		}
	})
}

func TestRemember(t *testing.T) {
	t.Run("given a working server when remembered then client.json holds it without the password", func(t *testing.T) {
		settings := filepath.Join(t.TempDir(), "client.json")
		c, _ := Connect(Options{Server: "10.0.0.9:8471", Password: "secret", Auth: "password"}, io.Discard)
		if err := Remember(settings, c, "password"); err != nil {
			t.Fatal(err)
		}
		saved, _ := config.LoadClientSettings(settings)
		if saved.Server != "http://10.0.0.9:8471" || saved.Auth != "password" {
			t.Fatalf("saved %+v", saved)
		}
	})
}
