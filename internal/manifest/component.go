package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"yapl/internal/config"
	"yapl/internal/paths"
)

// ComponentArchiveFile is the archive name inside a stored component.
const ComponentArchiveFile = "component.tar.zst"

// ComponentFormat is the compression used for component archives: zstd
// decompresses multi-GB Proton builds far faster than xz.
const ComponentFormat = "zst"

// ComponentKinds lists every shareable component kind.
var ComponentKinds = []string{paths.KindProton, paths.KindRuntime, paths.KindDXVK, paths.KindVKD3D, paths.KindUMU}

// ComponentManifest describes one immutable component stored in a registry.
// VersionInfo carries the runner.json entry minus machine-specific fields,
// so a client can register the component without its original URL.
type ComponentManifest struct {
	SchemaVersion int                `json:"schema_version"`
	Kind          string             `json:"kind"`
	Version       string             `json:"version"`
	Archive       Archive            `json:"archive"`
	VersionInfo   config.VersionInfo `json:"version_info"`
}

// ValidComponentKind reports whether kind is a shareable component kind.
func ValidComponentKind(kind string) bool { return slices.Contains(ComponentKinds, kind) }

// Validate checks the component manifest is complete and safe to act on.
func (m ComponentManifest) Validate() error {
	var errs []string
	if m.SchemaVersion < 1 || m.SchemaVersion > SchemaVersion {
		errs = append(errs, fmt.Sprintf("unsupported schema version %d", m.SchemaVersion))
	}
	if !ValidComponentKind(m.Kind) {
		errs = append(errs, fmt.Sprintf("invalid kind %q: must be one of %s", m.Kind, strings.Join(ComponentKinds, ", ")))
	}
	if !ValidName(m.Version) {
		errs = append(errs, fmt.Sprintf("invalid version %q", m.Version))
	}
	if !ValidSHA256(m.Archive.SHA256) {
		errs = append(errs, fmt.Sprintf("invalid sha256 %q", m.Archive.SHA256))
	}
	if len(errs) > 0 {
		return errors.New("invalid component manifest: " + strings.Join(errs, "; "))
	}
	return nil
}

// Component returns the kind/version pair.
func (m ComponentManifest) Component() Component {
	return Component{Kind: m.Kind, Version: m.Version}
}

// PortableVersionInfo drops the fields that only make sense on the machine
// that downloaded the component.
func PortableVersionInfo(v config.VersionInfo) config.VersionInfo {
	v.URL = ""
	v.Path = ""
	v.CheckForUpdates = false
	return v
}

// ReadComponent loads and validates a component manifest file.
func ReadComponent(path string) (ComponentManifest, error) {
	var m ComponentManifest
	data, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("parse component manifest %s: %w", path, err)
	}
	return m, m.Validate()
}

// WriteComponent saves a component manifest as indented JSON.
func WriteComponent(path string, m ComponentManifest) error {
	return writeJSON(path, m)
}
