package console

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// DefaultStopGrace is how long a stopped game gets to exit after SIGTERM
// before its process group is killed.
const DefaultStopGrace = 5 * time.Second

const logFileMode = 0644

var (
	ErrSessionActive = errors.New("a game is already running")
	ErrNoSession     = errors.New("no game is running")
)

// Launcher builds the command that runs a game. The default runs
// `yapl --workdir <library> run <name>` as a separate process so a crashing
// game or Wine can never take the console down.
type Launcher func(name string) *exec.Cmd

// Exit records how the last session ended.
type Exit struct {
	Name  string    `json:"name"`
	Code  int       `json:"code"`
	Error string    `json:"error,omitempty"`
	Ended time.Time `json:"ended"`
}

// SessionStatus is what /api/session returns.
type SessionStatus struct {
	Running bool      `json:"running"`
	Name    string    `json:"name,omitempty"`
	Started time.Time `json:"started,omitempty"`
	PID     int       `json:"pid,omitempty"`
	Last    *Exit     `json:"last,omitempty"`
}

type running struct {
	name    string
	started time.Time
	cmd     *exec.Cmd
	exited  chan struct{}
}

// Sessions runs at most one game at a time.
type Sessions struct {
	mu       sync.Mutex
	launcher Launcher
	current  *running
	last     *Exit
	grace    time.Duration
}

// NewSessions creates a session manager using launcher.
func NewSessions(launcher Launcher) *Sessions {
	return &Sessions{launcher: launcher, grace: DefaultStopGrace}
}

// Launch starts name, writing its output to logPath.
func (s *Sessions) Launch(name, logPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current != nil {
		return fmt.Errorf("%w: %s", ErrSessionActive, s.current.name)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, logFileMode)
	if err != nil {
		return fmt.Errorf("open log: %w", err)
	}
	cmd := s.launcher(name)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	// Its own process group, so Stop reaches Wine's helper processes too.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("launch %s: %w", name, err)
	}
	r := &running{name: name, started: time.Now(), cmd: cmd, exited: make(chan struct{})}
	s.current = r
	go func() {
		waitErr := cmd.Wait()
		logFile.Close()
		exit := &Exit{Name: name, Code: cmd.ProcessState.ExitCode(), Ended: time.Now()}
		if waitErr != nil {
			exit.Error = waitErr.Error()
		}
		s.mu.Lock()
		s.current, s.last = nil, exit
		s.mu.Unlock()
		close(r.exited)
	}()
	return nil
}

// Status reports the running game and how the previous one ended.
func (s *Sessions) Status() SessionStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := SessionStatus{Last: s.last}
	if s.current != nil {
		st.Running, st.Name, st.Started, st.PID = true, s.current.name, s.current.started, s.current.cmd.Process.Pid
	}
	return st
}

// Stop asks the running game's process group to exit, killing it after the
// grace period, and waits until it has gone.
func (s *Sessions) Stop() error {
	s.mu.Lock()
	r := s.current
	s.mu.Unlock()
	if r == nil {
		return ErrNoSession
	}
	pgid := -r.cmd.Process.Pid
	syscall.Kill(pgid, syscall.SIGTERM)
	select {
	case <-r.exited:
	case <-time.After(s.grace):
		syscall.Kill(pgid, syscall.SIGKILL)
		<-r.exited
	}
	// The leader can exit before its children; make sure none linger.
	syscall.Kill(pgid, syscall.SIGKILL)
	return nil
}
