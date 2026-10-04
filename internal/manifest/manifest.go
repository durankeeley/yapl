// Package manifest describes a packaged game or app: who it is (metadata),
// what archive carries it (with checksum) and which shared components it
// needs. Registries, pulls and upgrades all work from manifests.
package manifest

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"yapl/internal/config"
	"yapl/internal/defaults"
	"yapl/internal/paths"
	"yapl/internal/version"
)

const (
	// SchemaVersion is bumped on incompatible manifest changes; readers
	// reject manifests newer than they understand.
	SchemaVersion = 1

	MetadataFile   = "metadata.json"
	DefaultVersion = "1.0.0"

	sidecarSuffix      = ".manifest.json"
	coverSidecarSuffix = ".cover"
	sha256HexLength    = 64
)

// CoverNames are the file names searched for cover art in a game directory.
var CoverNames = []string{"cover.jpg", "cover.png", "cover.webp"}

// Metadata is authored by the operator in games/<name>/metadata.json.
type Metadata struct {
	Title       string   `json:"title"`
	Version     string   `json:"version"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Players     string   `json:"players,omitempty"`
	Cover       string   `json:"cover,omitempty"`
	// Preserve lists globs, relative to the game directory, that survive
	// upgrades (saves and configs kept next to the executable).
	Preserve []string `json:"preserve,omitempty"`
}

// Component is a shared, immutable dependency such as a Proton build.
type Component struct {
	Kind    string `json:"kind"`
	Version string `json:"version"`
}

func (c Component) String() string { return c.Kind + "/" + c.Version }

// Archive describes the package file itself.
type Archive struct {
	File      string `json:"file"`
	Format    string `json:"format"`
	SHA256    string `json:"sha256,omitempty"`
	SizeBytes int64  `json:"size_bytes"`
}

// Manifest is everything a registry or client needs to know about a package.
type Manifest struct {
	SchemaVersion int         `json:"schema_version"`
	Name          string      `json:"name"`
	Type          string      `json:"type"`
	Metadata      Metadata    `json:"metadata"`
	Archive       Archive     `json:"archive"`
	Components    []Component `json:"components,omitempty"`
	Bundled       bool        `json:"bundled,omitempty"`
	CreatedBy     string      `json:"created_by,omitempty"`
}

// DefaultMetadata is used when a game has no metadata.json yet.
func DefaultMetadata(name string) Metadata {
	return Metadata{Title: name, Version: DefaultVersion}
}

// LoadMetadata reads appDir/metadata.json, filling missing title and version
// from defaults. existed is false when the file is absent.
func LoadMetadata(appDir, name string) (m Metadata, existed bool, err error) {
	data, err := os.ReadFile(filepath.Join(appDir, MetadataFile))
	if os.IsNotExist(err) {
		return DefaultMetadata(name), false, nil
	}
	if err != nil {
		return Metadata{}, false, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return Metadata{}, true, fmt.Errorf("parse %s: %w", MetadataFile, err)
	}
	if m.Title == "" {
		m.Title = name
	}
	if m.Version == "" {
		m.Version = DefaultVersion
	}
	return m, true, nil
}

// SaveMetadata writes appDir/metadata.json.
func SaveMetadata(appDir string, m Metadata) error {
	return writeJSON(filepath.Join(appDir, MetadataFile), m)
}

// CoverPath returns the cover image in appDir, or "" when there is none.
func (m Metadata) CoverPath(appDir string) string {
	names := CoverNames
	if m.Cover != "" {
		names = append([]string{m.Cover}, names...)
	}
	for _, n := range names {
		p := filepath.Join(appDir, n)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

// ComponentsFor lists the shared components a config needs at run time.
func ComponentsFor(cfg config.App) []Component {
	var out []Component
	if cfg.ProtonVersion != "" && cfg.ProtonVersion != "system" {
		out = append(out, Component{Kind: paths.KindProton, Version: cfg.ProtonVersion})
	}
	method := cfg.LaunchMethod
	if method == "" {
		method = defaults.LaunchMethod
	}
	if method == "container" && cfg.RuntimeVersion != "" {
		out = append(out, Component{Kind: paths.KindRuntime, Version: cfg.RuntimeVersion})
	}
	if method == "umu" && !cfg.UMUOptions.UseSystemBinary && cfg.UMUOptions.Version != "" {
		out = append(out, Component{Kind: paths.KindUMU, Version: cfg.UMUOptions.Version})
	}
	if v := cfg.Dependencies.DXVKVersion; v != "" {
		out = append(out, Component{Kind: paths.KindDXVK, Version: v})
	}
	if v := cfg.Dependencies.VKD3DVersion; v != "" {
		out = append(out, Component{Kind: paths.KindVKD3D, Version: v})
	}
	return out
}

// ValidName reports whether s is safe as a package, component or version
// name: it can be used as one path segment and in URLs without escaping
// tricks.
func ValidName(s string) bool {
	if s == "" || strings.HasPrefix(s, ".") || strings.ContainsAny(s, "/\\\x00") || strings.Contains(s, "..") {
		return false
	}
	return true
}

// Validate checks the manifest is complete and safe to act on.
func (m Manifest) Validate() error {
	var errs []string
	if m.SchemaVersion < 1 || m.SchemaVersion > SchemaVersion {
		errs = append(errs, fmt.Sprintf("unsupported schema version %d (this yapl understands up to %d)", m.SchemaVersion, SchemaVersion))
	}
	if !ValidName(m.Name) {
		errs = append(errs, fmt.Sprintf("invalid name %q", m.Name))
	}
	if m.Type != paths.KindGame && m.Type != paths.KindApp {
		errs = append(errs, fmt.Sprintf("invalid type %q: must be game or app", m.Type))
	}
	if !version.Valid(m.Metadata.Version) {
		errs = append(errs, fmt.Sprintf("invalid version %q: must be MAJOR.MINOR.PATCH", m.Metadata.Version))
	}
	if !ValidSHA256(m.Archive.SHA256) && m.Archive.SHA256 != "" {
		errs = append(errs, fmt.Sprintf("invalid sha256 %q", m.Archive.SHA256))
	}
	for _, c := range m.Components {
		if !ValidName(c.Kind) || !ValidName(c.Version) {
			errs = append(errs, fmt.Sprintf("invalid component %q", c.String()))
		}
	}
	if len(errs) > 0 {
		return errors.New("invalid manifest: " + strings.Join(errs, "; "))
	}
	return nil
}

// ValidSHA256 reports whether s is a hex-encoded SHA-256 digest.
func ValidSHA256(s string) bool {
	if len(s) != sha256HexLength {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// Read loads and validates a manifest file.
func Read(path string) (Manifest, error) {
	var m Manifest
	data, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	return m, m.Validate()
}

// Write saves a manifest as indented JSON.
func Write(path string, m Manifest) error {
	return writeJSON(path, m)
}

// SidecarPath is where `package` writes the manifest next to the archive.
func SidecarPath(dir, name string) string {
	return filepath.Join(dir, name+sidecarSuffix)
}

// CoverSidecarPath is where `package` copies the cover next to the archive.
func CoverSidecarPath(dir, name, ext string) string {
	return filepath.Join(dir, name+coverSidecarSuffix+ext)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}
