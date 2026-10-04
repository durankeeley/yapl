// Package defaults holds every shared tunable in one place, so no port,
// timeout or format is a magic literal elsewhere in YAPL.
package defaults

import "time"

const (
	// ServerPort is used for both the registry's HTTP API and its UDP
	// discovery broadcast.
	ServerPort = 8471
	// ConsolePort is the loopback port for the console UI and its API.
	ConsolePort = 8472

	// DiscoveryTimeout bounds how long a client waits for a server broadcast;
	// it must exceed BroadcastInterval so at least one broadcast is heard.
	DiscoveryTimeout = 3 * time.Second
	// BroadcastInterval is how often a server announces itself on the LAN.
	BroadcastInterval = 2 * time.Second
	// ManifestRefreshInterval is how often a server rescans its packages
	// directory for files copied in by hand.
	ManifestRefreshInterval = 30 * time.Second
	// DownloadTimeout caps a single HTTP download (multi-GB Proton builds
	// over slow links need the headroom).
	DownloadTimeout = 30 * time.Minute

	PackageFormat = "xz"
	LaunchMethod  = "container"
	AuthMode      = "open"
)

// LaunchMethods lists every supported launch_method value.
var LaunchMethods = []string{"direct", "container", "umu"}

// PackageFormats lists every supported package compression format.
var PackageFormats = []string{"gz", "xz", "zst"}
