package command

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// All tests use real shell scripts as stand-in "Windows executables" so behaviour
// can be proven without Wine. The runner binary is "sh" and services are shell
// scripts that write their PID to a file then sleep — giving us a concrete handle
// on the process lifecycle.

func TestLaunchBackgroundServices_EmptyListIsNoop(t *testing.T) {
	// Given no background services configured
	// When launchBackgroundServices is called with an empty list
	cleanup := launchBackgroundServices(nil, "sh", nil, os.Environ(), t.TempDir(), 0)

	// Then cleanup can be called without panic, and no delay occurred
	cleanup()
}

func TestLaunchBackgroundServices_ServiceIsAliveAfterStart(t *testing.T) {
	// Given a service script that records its PID and runs indefinitely
	prefixDir := t.TempDir()
	pidFile := filepath.Join(prefixDir, "svc.pid")
	writeSleepScript(t, filepath.Join(prefixDir, "svc.sh"), pidFile)

	// When launchBackgroundServices is called with a short startup delay
	cleanup := launchBackgroundServices(
		[]string{"svc.sh"}, "sh", nil, os.Environ(), prefixDir, 50,
	)
	defer cleanup()

	// Then the service process is running
	pid := pollForPID(t, pidFile)
	assertAlive(t, pid)
}

func TestLaunchBackgroundServices_CleanupKillsService(t *testing.T) {
	// Given a running background service
	prefixDir := t.TempDir()
	pidFile := filepath.Join(prefixDir, "svc.pid")
	writeSleepScript(t, filepath.Join(prefixDir, "svc.sh"), pidFile)

	cleanup := launchBackgroundServices(
		[]string{"svc.sh"}, "sh", nil, os.Environ(), prefixDir, 50,
	)

	pid := pollForPID(t, pidFile)
	assertAlive(t, pid)

	// When cleanup is called (simulating game exit)
	cleanup()
	time.Sleep(50 * time.Millisecond)

	// Then the service is no longer running
	assertDead(t, pid)
}

func TestLaunchBackgroundServices_MultipleServicesAllKilledByCleanup(t *testing.T) {
	// Given two background services — the Starcraft LAN case: relay + lobby service
	prefixDir := t.TempDir()
	pidFile1 := filepath.Join(prefixDir, "svc1.pid")
	pidFile2 := filepath.Join(prefixDir, "svc2.pid")
	writeSleepScript(t, filepath.Join(prefixDir, "svc1.sh"), pidFile1)
	writeSleepScript(t, filepath.Join(prefixDir, "svc2.sh"), pidFile2)

	cleanup := launchBackgroundServices(
		[]string{"svc1.sh", "svc2.sh"}, "sh", nil, os.Environ(), prefixDir, 50,
	)

	pid1 := pollForPID(t, pidFile1)
	pid2 := pollForPID(t, pidFile2)
	assertAlive(t, pid1)
	assertAlive(t, pid2)

	// When the game exits and cleanup runs
	cleanup()
	time.Sleep(50 * time.Millisecond)

	// Then both services are dead
	assertDead(t, pid1)
	assertDead(t, pid2)
}

func TestLaunchBackgroundServices_NonExistentRunnerDoesNotPanic(t *testing.T) {
	// Given a runner binary path that does not exist (simulates Wine not found)
	prefixDir := t.TempDir()
	writeSleepScript(t, filepath.Join(prefixDir, "svc.sh"), filepath.Join(prefixDir, "svc.pid"))

	// When launchBackgroundServices is called — cmd.Start() will fail
	cleanup := launchBackgroundServices(
		[]string{"svc.sh"}, "/nonexistent/runner-binary", nil, os.Environ(), prefixDir, 0,
	)

	// Then it does not panic and cleanup is safe to call
	cleanup()
}

func TestLaunchBackgroundServices_ArgsPrefixIsPassedBeforeServicePath(t *testing.T) {
	// Given an argsPrefix that, when used with "sh" as the runner, creates a marker file.
	// This simulates container mode where the entryPointPath receives:
	//   ["--verb=waitforexitandrun", "--", shimPath, protonScriptPath, "waitforexitandrun", <exe>]
	//
	// We verify the prefix arrives before the service path by using:
	//   sh -c "touch <marker> && sleep 30" <svc-full-path>
	// In sh's -c mode, $0 is set to <svc-full-path> (script name, not executed).
	// If args were reversed the shell would try to run the svc script directly,
	// skipping the -c command and leaving the marker uncreated.
	prefixDir := t.TempDir()
	markerFile := filepath.Join(prefixDir, "prefix-args-received")

	// The service path exists but is only used as $0 — it is not executed directly.
	svcScript := "svc.sh"
	if err := os.WriteFile(filepath.Join(prefixDir, svcScript), []byte("#!/bin/sh\nsleep 30\n"), 0755); err != nil {
		t.Fatal(err)
	}

	cleanup := launchBackgroundServices(
		[]string{svcScript},
		"sh",
		[]string{"-c", "touch " + markerFile + " && sleep 30"},
		os.Environ(),
		prefixDir,
		100,
	)
	defer cleanup()

	// Then the marker file was created, proving argsPrefix was passed before the service path
	if _, err := os.Stat(markerFile); err != nil {
		t.Fatal("argsPrefix was not passed before the service path — marker file was not created")
	}
}

// --- test helpers ---

// writeSleepScript creates an executable shell script that records its own PID
// then sleeps indefinitely. The PID file is used to verify liveness and death.
func writeSleepScript(t *testing.T, scriptPath, pidFile string) {
	t.Helper()
	content := "#!/bin/sh\necho $$ > " + pidFile + "\nsleep 30\n"
	if err := os.WriteFile(scriptPath, []byte(content), 0755); err != nil {
		t.Fatalf("could not write service script: %v", err)
	}
}

// pollForPID reads the PID file written by writeSleepScript, polling until it
// appears or a 2-second deadline passes. Returns the PID as an int.
func pollForPID(t *testing.T, pidFile string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(pidFile)
		if err == nil {
			trimmed := strings.TrimSpace(string(data))
			if trimmed != "" {
				pid, err := strconv.Atoi(trimmed)
				if err != nil {
					t.Fatalf("PID file %q contained non-integer %q: %v", pidFile, trimmed, err)
				}
				return pid
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("service did not write PID to %q within 2 seconds", pidFile)
	return 0
}

// assertAlive fails the test if the process with the given PID is not running.
func assertAlive(t *testing.T, pid int) {
	t.Helper()
	proc, err := os.FindProcess(pid)
	if err != nil {
		t.Fatalf("could not find process %d: %v", pid, err)
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("expected process %d to be alive, but signal(0) returned: %v", pid, err)
	}
}

// assertDead fails the test if the process with the given PID is still running.
func assertDead(t *testing.T, pid int) {
	t.Helper()
	proc, _ := os.FindProcess(pid)
	if err := proc.Signal(syscall.Signal(0)); err == nil {
		t.Fatalf("expected process %d to be dead after cleanup, but it is still alive", pid)
	}
}
