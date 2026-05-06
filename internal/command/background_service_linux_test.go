package command

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"yapl/internal/config"
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

// --- ordering guarantee ---

func TestLaunchBackgroundServices_ServiceAliveWhenMainStarts(t *testing.T) {
	// The central contract: when launchBackgroundServices returns, the caller can
	// immediately run the main executable and the services will already be up.
	// This is the Starcraft LAN scenario: relay must accept connections before the
	// game opens its socket — the delay window is what makes that possible.
	prefixDir := t.TempDir()
	svcPIDFile := filepath.Join(prefixDir, "svc.pid")
	resultFile := filepath.Join(prefixDir, "result.txt")

	writeSleepScript(t, filepath.Join(prefixDir, "svc.sh"), svcPIDFile)

	// A script that plays the role of the main game: checks whether the service is
	// alive at the exact moment it runs, and writes the verdict to a file.
	mainScript := filepath.Join(prefixDir, "main.sh")
	if err := os.WriteFile(mainScript, []byte("#!/bin/sh\n"+
		`pid=$(cat "`+svcPIDFile+`" 2>/dev/null || echo 0)`+"\n"+
		`if kill -0 "$pid" 2>/dev/null; then`+"\n"+
		`    echo "service_alive" > "`+resultFile+`"`+"\n"+
		`else`+"\n"+
		`    echo "service_dead" > "`+resultFile+`"`+"\n"+
		`fi`+"\n",
	), 0755); err != nil {
		t.Fatal(err)
	}

	// Start background services — this is what RunDirectly does before executeCommand
	cleanup := launchBackgroundServices(
		[]string{"svc.sh"}, "sh", nil, os.Environ(), prefixDir, 100,
	)
	defer cleanup()

	// Run main exe — this is executeCommand in RunDirectly
	if err := exec.Command("sh", mainScript).Run(); err != nil {
		t.Fatalf("main script failed: %v", err)
	}

	result, err := os.ReadFile(resultFile)
	if err != nil {
		t.Fatal("main script did not write result file")
	}
	if strings.TrimSpace(string(result)) != "service_alive" {
		t.Fatalf("expected service to be alive when main started, got %q", strings.TrimSpace(string(result)))
	}
}

// --- delay timing ---

func TestLaunchBackgroundServices_DelayElapsesBefore_Return(t *testing.T) {
	// The delay must elapse before launchBackgroundServices returns.
	// If it returned immediately the main exe could start before the service is
	// ready to accept connections.
	prefixDir := t.TempDir()
	writeSleepScript(t, filepath.Join(prefixDir, "svc.sh"), filepath.Join(prefixDir, "svc.pid"))

	const delayMs = 100
	start := time.Now()
	cleanup := launchBackgroundServices(
		[]string{"svc.sh"}, "sh", nil, os.Environ(), prefixDir, delayMs,
	)
	elapsed := time.Since(start)
	defer cleanup()

	if elapsed < time.Duration(delayMs)*time.Millisecond {
		t.Fatalf("expected at least %dms delay before return but returned after %v", delayMs, elapsed)
	}
}

func TestLaunchBackgroundServices_NoDelayForEmptyServiceList(t *testing.T) {
	// When there are no background services the caller must not be penalised with a
	// startup delay — games that don't need a service should start immediately.
	const wouldWaitMs = 5000
	start := time.Now()
	cleanup := launchBackgroundServices(nil, "sh", nil, os.Environ(), t.TempDir(), wouldWaitMs)
	elapsed := time.Since(start)
	cleanup()

	if elapsed > 500*time.Millisecond {
		t.Fatalf("expected no delay for empty service list but took %v", elapsed)
	}
}

func TestLaunchBackgroundServices_NoDelayWhenAllServicesFail(t *testing.T) {
	// When all services fail to start (e.g. runner binary not found) no delay should
	// be applied — nothing started means nothing to wait for.
	prefixDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(prefixDir, "svc.sh"), []byte("#!/bin/sh\nsleep 30\n"), 0755); err != nil {
		t.Fatal(err)
	}

	const wouldWaitMs = 5000
	start := time.Now()
	cleanup := launchBackgroundServices(
		[]string{"svc.sh"}, "/nonexistent/runner", nil, os.Environ(), prefixDir, wouldWaitMs,
	)
	elapsed := time.Since(start)
	cleanup()

	if elapsed > 500*time.Millisecond {
		t.Fatalf("expected no delay when all services fail to start but took %v", elapsed)
	}
}

// --- integration: RunDirectly wires background services correctly ---

func TestRunDirectly_BackgroundServiceIsAliveWhenMainStarts(t *testing.T) {
	// End-to-end proof through RunDirectly using a fake wine64 binary.
	// The fake wine64 dispatches on its first argument:
	//   - *svc.exe  → writes its PID and sleeps (simulates the LAN relay service)
	//   - *main.exe → checks whether the service is alive, writes verdict, exits 0
	//
	// If RunDirectly starts the main exe before (or without) starting the service,
	// the result file will say "service_dead" and the test fails.
	protonDir := t.TempDir()
	binDir := filepath.Join(protonDir, "files", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}

	prefixDir := t.TempDir()
	svcPIDFile := filepath.Join(prefixDir, "svc.pid")
	resultFile := filepath.Join(prefixDir, "result.txt")

	wineScript := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"  *svc.exe)\n" +
		"    echo $$ > \"" + svcPIDFile + "\"\n" +
		"    sleep 30\n" +
		"    ;;\n" +
		"  *main.exe)\n" +
		"    pid=$(cat \"" + svcPIDFile + "\" 2>/dev/null || echo 0)\n" +
		"    if kill -0 \"$pid\" 2>/dev/null; then\n" +
		"        echo \"service_alive\" > \"" + resultFile + "\"\n" +
		"    else\n" +
		"        echo \"service_dead\" > \"" + resultFile + "\"\n" +
		"    fi\n" +
		"    exit 0\n" +
		"    ;;\n" +
		"  *)\n" +
		"    exit 0\n" +
		"    ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(binDir, "wine64"), []byte(wineScript), 0755); err != nil {
		t.Fatal(err)
	}

	globalCfg := config.Global{
		ProtonVersions: map[string]config.VersionInfo{
			"fake-proton": {Path: protonDir},
		},
	}

	// Placeholder exe files: wine64 receives their full paths as arguments
	for _, dir := range []string{
		filepath.Join(prefixDir, "drive_c", "services"),
		filepath.Join(prefixDir, "drive_c", "game"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(prefixDir, "drive_c", "services", "svc.exe"), nil, 0644)
	os.WriteFile(filepath.Join(prefixDir, "drive_c", "game", "main.exe"), nil, 0644)

	appCfg := config.App{
		ProtonVersion:            "fake-proton",
		LaunchMethod:             "direct",
		Executable:               "drive_c/game/main.exe",
		BackgroundServices:       []string{"drive_c/services/svc.exe"},
		BackgroundServiceDelayMs: 100,
	}

	if err := RunDirectly(prefixDir, appCfg, globalCfg, false, false); err != nil {
		t.Fatalf("RunDirectly returned unexpected error: %v", err)
	}

	result, err := os.ReadFile(resultFile)
	if err != nil {
		t.Fatalf("main exe did not write result file — service may not have been started: %v", err)
	}
	if strings.TrimSpace(string(result)) != "service_alive" {
		t.Fatalf("expected service to be alive when main started, got %q", strings.TrimSpace(string(result)))
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
