package registry

import (
	"bytes"
	"strings"
	"testing"

	"yapl/internal/kernel"
)

func execute(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	k := kernel.New("yapl", &out, &errOut)
	if err := k.Register(Module{}); err != nil {
		t.Fatal(err)
	}
	return k.Execute(args), errOut.String()
}

func TestServe(t *testing.T) {
	t.Run("given no packages directory when serve runs then it is a usage error", func(t *testing.T) {
		// When
		code, errOut := execute(t, "serve")

		// Then
		if code != kernel.ExitUsage || !strings.Contains(errOut, "--packages-dir is required") {
			t.Fatalf("exit %d err:\n%s", code, errOut)
		}
	})

	t.Run("given an unknown auth mode when serve runs then it fails before listening", func(t *testing.T) {
		// When
		code, errOut := execute(t, "serve", "--packages-dir", t.TempDir(), "--auth", "magic")

		// Then
		if code != kernel.ExitError || !strings.Contains(errOut, "unknown auth mode") {
			t.Fatalf("exit %d err:\n%s", code, errOut)
		}
	})
}
