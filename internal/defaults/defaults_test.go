package defaults

import (
	"slices"
	"testing"
)

func TestDefaults(t *testing.T) {
	t.Run("given the server and console ports then they never collide", func(t *testing.T) {
		if ServerPort == ConsolePort {
			t.Fatal("server and console ports must differ")
		}
	})
	t.Run("given discovery and broadcast timing then a client always hears at least one broadcast", func(t *testing.T) {
		if DiscoveryTimeout <= BroadcastInterval {
			t.Fatalf("discovery timeout %v must exceed broadcast interval %v", DiscoveryTimeout, BroadcastInterval)
		}
	})
	t.Run("given the default format and method then they are supported values", func(t *testing.T) {
		if !slices.Contains(PackageFormats, PackageFormat) || !slices.Contains(LaunchMethods, LaunchMethod) {
			t.Fatal("defaults must be members of their supported lists")
		}
	})
}
