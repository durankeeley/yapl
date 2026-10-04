// Package store indexes a registry's packages directory: versioned packages
// written by push, plus flat archives copied in by hand.
//
// Layout under the root:
//
//	games/<Name>/<version>/<Name>.tar.<fmt>   versioned package
//	games/<Name>/<version>/manifest.json
//	games/<Name>/<version>/cover.<ext>
//	games/<Name>.tar.<fmt>                    legacy drop-in
//	games/<Name>.manifest.json                optional legacy sidecar
//	games/<Name>.cover.<ext>                  optional legacy cover
package store

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"yapl/internal/archive"
	"yapl/internal/manifest"
	"yapl/internal/paths"
	"yapl/internal/version"
)

const (
	ManifestFile  = "manifest.json"
	ComponentsDir = "components"
	// LatestTag resolves to the highest version of a package.
	LatestTag = manifest.LatestTag
	// LegacyVersion is reported for flat archives that have no sidecar.
	LegacyVersion = manifest.LegacyVersion

	coverPrefix = "cover"
)

var (
	ErrNotFound    = errors.New("not found")
	ErrInvalidName = errors.New("invalid name")
)

// typeDirs is scanned in order; the first type to claim a name keeps it.
var typeDirs = []string{paths.GamesDir, paths.AppsDir}

// Version is one stored version of a package.
type Version struct {
	Manifest    manifest.Manifest
	ArchivePath string
	CoverPath   string
}

// Summary is one catalog row: a package and its latest metadata.
type Summary struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Players     string   `json:"players,omitempty"`
	Latest      string   `json:"latest"`
	Versions    []string `json:"versions"`
	Format      string   `json:"format"`
	SizeBytes   int64    `json:"size_bytes"`
	HasCover    bool     `json:"has_cover"`
}

// ComponentVersion is one stored component.
type ComponentVersion struct {
	Manifest    manifest.ComponentManifest
	ArchivePath string
}

type entry struct {
	kind     string
	versions map[string]*Version
}

// Store is a concurrency-safe index of a packages directory.
type Store struct {
	root       string
	warnings   io.Writer
	mu         sync.RWMutex
	packages   map[string]*entry
	components map[string]*ComponentVersion
}

// New creates a store over root. Problems found while scanning (invalid
// manifests, name collisions) are written to warnings.
func New(root string, warnings io.Writer) *Store {
	return &Store{root: root, warnings: warnings, packages: map[string]*entry{}, components: map[string]*ComponentVersion{}}
}

// Root is the directory the store indexes.
func (s *Store) Root() string { return s.root }

// Refresh rescans the directory and atomically replaces the index.
func (s *Store) Refresh() error {
	next := map[string]*entry{}
	for _, typeDir := range typeDirs {
		kind := paths.KindOf(typeDir)
		dir := filepath.Join(s.root, typeDir)
		items, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		for _, item := range items {
			if strings.HasPrefix(item.Name(), ".") {
				continue
			}
			var name string
			var found []*Version
			if item.IsDir() {
				name = item.Name()
				found = s.scanVersioned(dir, name, kind)
			} else if v, ok := s.scanLegacy(dir, item.Name(), kind); ok {
				name = v.Manifest.Name
				found = []*Version{v}
			}
			if len(found) == 0 {
				continue
			}
			if e, exists := next[name]; exists && e.kind != kind {
				fmt.Fprintf(s.warnings, "warning: %q exists in both games/ and apps/; keeping the %s\n", name, e.kind)
				continue
			}
			if next[name] == nil {
				next[name] = &entry{kind: kind, versions: map[string]*Version{}}
			}
			for _, v := range found {
				next[name].versions[v.Manifest.Metadata.Version] = v
			}
		}
	}
	components := s.scanComponents()
	s.mu.Lock()
	s.packages = next
	s.components = components
	s.mu.Unlock()
	return nil
}

func (s *Store) scanComponents() map[string]*ComponentVersion {
	out := map[string]*ComponentVersion{}
	for _, kind := range manifest.ComponentKinds {
		kindDir := filepath.Join(s.root, ComponentsDir, kind)
		versions, err := os.ReadDir(kindDir)
		if err != nil {
			continue
		}
		for _, vd := range versions {
			if !vd.IsDir() || strings.HasPrefix(vd.Name(), ".") {
				continue
			}
			dir := filepath.Join(kindDir, vd.Name())
			m, err := manifest.ReadComponent(filepath.Join(dir, ManifestFile))
			if err == nil && (m.Kind != kind || m.Version != vd.Name()) {
				err = fmt.Errorf("manifest says %s, directory says %s/%s", m.Component(), kind, vd.Name())
			}
			if err != nil {
				fmt.Fprintf(s.warnings, "warning: skipping %s: %v\n", dir, err)
				continue
			}
			out[m.Component().String()] = &ComponentVersion{Manifest: m, ArchivePath: filepath.Join(dir, manifest.ComponentArchiveFile)}
		}
	}
	return out
}

// ResolveComponent finds a stored component.
func (s *Store) ResolveComponent(kind, version string) (*ComponentVersion, error) {
	if !manifest.ValidComponentKind(kind) || !manifest.ValidName(version) {
		return nil, ErrInvalidName
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.components[manifest.Component{Kind: kind, Version: version}.String()]
	if !ok {
		return nil, fmt.Errorf("component %s/%s: %w", kind, version, ErrNotFound)
	}
	return c, nil
}

// Components lists every stored component, sorted by kind then version.
func (s *Store) Components() []manifest.ComponentManifest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]manifest.ComponentManifest, 0, len(s.components))
	for _, c := range s.components {
		out = append(out, c.Manifest)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return version.Compare(out[i].Version, out[j].Version) < 0
	})
	return out
}

func (s *Store) scanVersioned(typeDir, name, kind string) []*Version {
	if !manifest.ValidName(name) {
		return nil
	}
	versionDirs, err := os.ReadDir(filepath.Join(typeDir, name))
	if err != nil {
		return nil
	}
	var out []*Version
	for _, vd := range versionDirs {
		if !vd.IsDir() || strings.HasPrefix(vd.Name(), ".") {
			continue
		}
		dir := filepath.Join(typeDir, name, vd.Name())
		m, err := manifest.Read(filepath.Join(dir, ManifestFile))
		if err == nil && (m.Name != name || m.Metadata.Version != vd.Name() || m.Type != kind || !manifest.ValidName(m.Archive.File)) {
			err = fmt.Errorf("manifest says %s %s:%s, directory says %s %s:%s", m.Type, m.Name, m.Metadata.Version, kind, name, vd.Name())
		}
		if err != nil {
			fmt.Fprintf(s.warnings, "warning: skipping %s: %v\n", dir, err)
			continue
		}
		out = append(out, &Version{
			Manifest:    m,
			ArchivePath: filepath.Join(dir, m.Archive.File),
			CoverPath:   findCover(dir, coverPrefix),
		})
	}
	return out
}

func (s *Store) scanLegacy(typeDir, fileName, kind string) (*Version, bool) {
	name, format, ok := archive.ParseName(fileName)
	if !ok || !manifest.ValidName(name) {
		return nil, false
	}
	archivePath := filepath.Join(typeDir, fileName)
	info, err := os.Stat(archivePath)
	if err != nil {
		return nil, false
	}
	m := manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		Name:          name,
		Type:          kind,
		Metadata:      manifest.Metadata{Title: name, Version: LegacyVersion},
		Archive:       manifest.Archive{File: fileName, Format: format, SizeBytes: info.Size()},
	}
	sidecar := manifest.SidecarPath(typeDir, name)
	if _, err := os.Stat(sidecar); err == nil {
		sm, err := manifest.Read(sidecar)
		switch {
		case err != nil:
			fmt.Fprintf(s.warnings, "warning: ignoring %s: %v\n", sidecar, err)
		case sm.Name != name || sm.Archive.File != fileName:
			fmt.Fprintf(s.warnings, "warning: ignoring %s: it describes %s\n", sidecar, sm.Archive.File)
		default:
			m = sm
			m.Type = kind
		}
	}
	return &Version{Manifest: m, ArchivePath: archivePath, CoverPath: findCover(typeDir, name+".cover")}, true
}

// findCover returns dir/<prefix>.<ext> for the first supported image.
func findCover(dir, prefix string) string {
	for _, n := range manifest.CoverNames {
		p := filepath.Join(dir, prefix+filepath.Ext(n))
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// Resolve finds a package version; tag is a version or LatestTag.
func (s *Store) Resolve(name, tag string) (*Version, error) {
	if !manifest.ValidName(name) || !manifest.ValidName(tag) {
		return nil, ErrInvalidName
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.packages[name]
	if !ok {
		return nil, fmt.Errorf("package %q: %w", name, ErrNotFound)
	}
	if tag == LatestTag {
		tag = version.Latest(keys(e.versions))
	}
	v, ok := e.versions[tag]
	if !ok {
		return nil, fmt.Errorf("package %q version %q: %w", name, tag, ErrNotFound)
	}
	return v, nil
}

// Versions returns every version of a package, newest first.
func (s *Store) Versions(name string) ([]*Version, error) {
	if !manifest.ValidName(name) {
		return nil, ErrInvalidName
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.packages[name]
	if !ok {
		return nil, fmt.Errorf("package %q: %w", name, ErrNotFound)
	}
	out := make([]*Version, 0, len(e.versions))
	for _, tag := range newestFirst(keys(e.versions)) {
		out = append(out, e.versions[tag])
	}
	return out, nil
}

// Catalog lists every package with its latest metadata, sorted by name.
func (s *Store) Catalog() []Summary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Summary, 0, len(s.packages))
	for name, e := range s.packages {
		tags := newestFirst(keys(e.versions))
		latest := e.versions[tags[0]]
		md := latest.Manifest.Metadata
		out = append(out, Summary{
			Name: name, Type: e.kind, Title: md.Title, Description: md.Description, Tags: md.Tags, Players: md.Players,
			Latest: tags[0], Versions: tags,
			Format: latest.Manifest.Archive.Format, SizeBytes: latest.Manifest.Archive.SizeBytes,
			HasCover: latest.CoverPath != "",
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func keys(m map[string]*Version) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func newestFirst(tags []string) []string {
	sort.Slice(tags, func(i, j int) bool { return version.Compare(tags[i], tags[j]) > 0 })
	return tags
}
