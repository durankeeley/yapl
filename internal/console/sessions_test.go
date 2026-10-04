package console

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// scriptLauncher runs a shell snippet as the "game"; $1 is the game name.
func scriptLauncher(script string) Launcher {
	return func(name string) *exec.Cmd {
		return exec.Command("sh", "-c", script, "game", name)
	}
}

func waitStatus(t *testing.T, s *Sessions, running bool) SessionStatus {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if st := s.Status(); st.Running == running {
			return st
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("session never reached running=%v: %+v", running, s.Status())
	return SessionStatus{}
}

func TestSessions(t *testing.T) {
	t.Run("given a game when launched then it is running, its output is logged and its exit is recorded", func(t *testing.T) {
		// Given
		s := NewSessions(scriptLauncher(`echo "starting $1"; sleep 0.2; exit 3`))
		logPath := filepath.Join(t.TempDir(), "last-run.log")

		// When
		if err := s.Launch("Doom", logPath); err != nil {
			t.Fatal(err)
		}

		// Then
		running := s.Status()
		if !running.Running || running.Name != "Doom" || running.PID == 0 {
			t.Fatalf("status = %+v", running)
		}
		finished := waitStatus(t, s, false)
		if finished.Last == nil || finished.Last.Name != "Doom" || finished.Last.Code != 3 {
			t.Fatalf("last = %+v", finished.Last)
		}
		if log, _ := os.ReadFile(logPath); !strings.Contains(string(log), "starting Doom") {
			t.Fatalf("log = %q", log)
		}
	})

	t.Run("given a running game when another is launched then it is refused", func(t *testing.T) {
		s := NewSessions(scriptLauncher(`sleep 5`))
		if err := s.Launch("Doom", filepath.Join(t.TempDir(), "log")); err != nil {
			t.Fatal(err)
		}
		defer s.Stop()
		if err := s.Launch("Quake", filepath.Join(t.TempDir(), "log")); !errors.Is(err, ErrSessionActive) || !strings.Contains(err.Error(), "Doom") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("given a game with child processes when stopped then the whole process group ends", func(t *testing.T) {
		// Given a "game" that starts a child (like Wine starting wineserver)
		pidFile := filepath.Join(t.TempDir(), "child.pid")
		s := NewSessions(scriptLauncher(`sleep 30 & echo $! > ` + pidFile + `; wait`))
		s.grace = 200 * time.Millisecond
		if err := s.Launch("Doom", filepath.Join(t.TempDir(), "log")); err != nil {
			t.Fatal(err)
		}
		var childPID int
		deadline := time.Now().Add(2 * time.Second)
		for childPID == 0 && time.Now().Before(deadline) {
			data, _ := os.ReadFile(pidFile)
			childPID, _ = strconv.Atoi(strings.TrimSpace(string(data)))
			time.Sleep(10 * time.Millisecond)
		}

		// When
		if err := s.Stop(); err != nil {
			t.Fatal(err)
		}

		// Then
		waitStatus(t, s, false)
		time.Sleep(50 * time.Millisecond)
		if childPID == 0 || syscall.Kill(childPID, 0) == nil {
			t.Fatalf("child process %d survived stop", childPID)
		}
	})

	t.Run("given nothing running when stopped then it reports there is no session", func(t *testing.T) {
		s := NewSessions(scriptLauncher(`true`))
		if err := s.Stop(); !errors.Is(err, ErrNoSession) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("given a launcher that cannot start when launched then it fails and nothing is running", func(t *testing.T) {
		s := NewSessions(func(string) *exec.Cmd { return exec.Command("/nonexistent/yapl") })
		if err := s.Launch("Doom", filepath.Join(t.TempDir(), "log")); err == nil {
			t.Fatal("expected an error")
		}
		if s.Status().Running {
			t.Fatal("session marked running")
		}
	})
}
