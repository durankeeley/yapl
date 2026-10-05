// Package console is the service behind `yapl console`: a loopback JSON API
// over four small parts — the local library, the registry store, a job
// queue for installs and upgrades, and game sessions.
package console

import (
	"errors"
	"os"
	"path/filepath"
	"sort"

	"yapl/internal/config"
	"yapl/internal/fs"
	"yapl/internal/manifest"
	"yapl/internal/paths"
)

// ErrNotInstalled is returned for names that are not in the library.
var ErrNotInstalled = errors.New("not installed")

// Game is one library entry as the console shows it.
type Game struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Players     string   `json:"players,omitempty"`
	Version     string   `json:"version"`
	Source      string   `json:"source,omitempty"`
	HasCover    bool     `json:"has_cover"`
	// Ready is false while the config still points at the placeholder
	// executable, i.e. the game was set up but never installed.
	Ready bool `json:"ready"`
}

// ListLibrary describes every configured game and app, games first, each
// sorted by name.
func ListLibrary() ([]Game, error) {
	var out []Game
	for _, typeDir := range []string{paths.GamesDir, paths.AppsDir} {
		entries, err := os.ReadDir(typeDir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var games []Game
		for _, e := range entries {
			if !fs.IsDir(filepath.Join(typeDir, e.Name())) {
				continue
			}
			if g, _, ok := describe(typeDir, e.Name()); ok {
				games = append(games, g)
			}
		}
		sort.Slice(games, func(i, j int) bool { return games[i].Name < games[j].Name })
		out = append(out, games...)
	}
	return out, nil
}

// FindGame describes one library entry and returns its cover path ("" when
// it has none).
func FindGame(name string) (Game, string, error) {
	if !manifest.ValidName(name) {
		return Game{}, "", ErrNotInstalled
	}
	for _, typeDir := range []string{paths.GamesDir, paths.AppsDir} {
		if g, cover, ok := describe(typeDir, name); ok {
			return g, cover, nil
		}
	}
	return Game{}, "", ErrNotInstalled
}

// Uninstall deletes a game or app directory.
func Uninstall(name string) error {
	g, _, err := FindGame(name)
	if err != nil {
		return err
	}
	return os.RemoveAll(paths.AppDir(paths.TypeDir(g.Type), g.Name))
}

func describe(typeDir, name string) (Game, string, bool) {
	dir := paths.AppDir(typeDir, name)
	cfg, err := config.LoadApp(filepath.Join(dir, paths.DefaultConfigName(typeDir)))
	if err != nil {
		return Game{}, "", false
	}
	meta, _, _ := manifest.LoadMetadata(dir, name)
	g := Game{
		Name: name, Type: paths.KindOf(typeDir), Title: meta.Title, Description: meta.Description,
		Tags: meta.Tags, Players: meta.Players, Version: meta.Version,
		Ready: cfg.Executable != "" && cfg.Executable != config.PlaceholderExecutable,
	}
	if in, err := manifest.ReadInstalled(dir); err == nil {
		g.Version = in.Version
		g.Source = in.Server
	}
	cover := meta.CoverPath(dir)
	g.HasCover = cover != ""
	return g, cover, true
}
