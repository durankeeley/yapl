package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"yapl/internal/version"
)

// InstalledFile records where an installed package came from.
const InstalledFile = "installed.json"

// LatestTag names the newest version of a package in a reference.
const LatestTag = "latest"

// LegacyVersion stands in for packages that predate manifests.
const LegacyVersion = "0.0.0"

// Installed is written into a game directory by pull and read by upgrade.
type Installed struct {
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Version     string    `json:"version"`
	Server      string    `json:"server,omitempty"`
	SHA256      string    `json:"sha256,omitempty"`
	InstalledAt time.Time `json:"installed_at"`
}

// ReadInstalled loads appDir/installed.json.
func ReadInstalled(appDir string) (Installed, error) {
	var in Installed
	data, err := os.ReadFile(filepath.Join(appDir, InstalledFile))
	if err != nil {
		return in, err
	}
	return in, json.Unmarshal(data, &in)
}

// WriteInstalled saves appDir/installed.json, stamping the install time
// when it is unset.
func WriteInstalled(appDir string, in Installed) error {
	if in.InstalledAt.IsZero() {
		in.InstalledAt = time.Now().UTC()
	}
	return writeJSON(filepath.Join(appDir, InstalledFile), in)
}

// ParseReference splits "name[:tag]". The text after the last colon is a
// tag only when it is a version or "latest", so names such as
// "Need for Speed: Most Wanted" survive intact.
func ParseReference(ref string) (name, tag string) {
	if i := strings.LastIndex(ref, ":"); i > 0 {
		if t := ref[i+1:]; t == LatestTag || version.Valid(t) {
			return ref[:i], t
		}
	}
	return ref, LatestTag
}
