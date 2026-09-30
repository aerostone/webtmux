package tmux

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/creack/pty"

	"github.com/aerostone/webtmux/internal/logger"
)

type Manager struct {
	socket string
}

func NewManager(socket string) *Manager {
	return &Manager{socket: socket}
}

func (m *Manager) ListSessions() ([]Session, error) {
	args := m.tmuxArgs("list-sessions",
		"-F", "#{session_name}|#{session_attached}|#{session_windows}")
	out, err := exec.Command("tmux", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("tmux list-sessions: %w", err)
	}
	return parseSessions(string(out)), nil
}

func (m *Manager) CreateSession(name string) error {
	args := m.tmuxArgs("new-session", "-d", "-s", name)
	if err := exec.Command("tmux", args...).Run(); err != nil {
		return fmt.Errorf("tmux new-session %q: %w", name, err)
	}
	return nil
}

func (m *Manager) KillSession(name string) error {
	args := m.tmuxArgs("kill-session", "-t", name)
	if err := exec.Command("tmux", args...).Run(); err != nil {
		return fmt.Errorf("tmux kill-session %q: %w", name, err)
	}
	return nil
}

// Resizable is an io.ReadWriteCloser that supports terminal resize.
// Done reports when the underlying child process has been reaped (Wait
// returned) and is safe to wait on during connection replacement.
type Resizable interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
	PID() int
	Done() <-chan struct{}
}

func (m *Manager) AttachSession(name string) (Resizable, error) {
	args := m.tmuxArgs("attach-session", "-t", name)
	cmd := exec.Command("tmux", args...)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("tmux attach %q: %w", name, err)
	}
	c := &ptyConn{ptmx: ptmx, cmd: cmd, done: make(chan struct{})}
	// Reap the child as soon as it exits. Calling Process.Kill without a
	// matching Wait would leave a zombie + pidfd behind on every detach,
	// accumulating without bound.
	go func() {
		defer close(c.done)
		c.cmd.Wait()
	}()
	return c, nil
}

func (m *Manager) tmuxArgs(parts ...string) []string {
	if m.socket != "" {
		return append([]string{"-L", m.socket}, parts...)
	}
	return parts
}

type ptyConn struct {
	ptmx *os.File
	cmd  *exec.Cmd
	done chan struct{} // closed by the reaper goroutine after cmd.Wait() returns

	closeOnce sync.Once
}

func (c *ptyConn) Read(p []byte) (int, error)  { return c.ptmx.Read(p) }
func (c *ptyConn) Write(p []byte) (int, error) { return c.ptmx.Write(p) }
func (c *ptyConn) Close() error {
	// First Close wins: close pty (unblocks Read pump) then kill the child.
	// The reaper goroutine started in AttachSession performs the matching
	// Wait(), so the child is always reaped exactly once regardless of how
	// many times Close is called or who killed the child first.
	c.closeOnce.Do(func() {
		c.ptmx.Close()
		if c.cmd.Process != nil {
			c.cmd.Process.Kill()
		}
	})
	return nil
}

func (c *ptyConn) PID() int {
	if c.cmd.Process != nil {
		return c.cmd.Process.Pid
	}
	return 0
}

// Done is closed once the child process has exited and been reaped.
func (c *ptyConn) Done() <-chan struct{} { return c.done }

func (c *ptyConn) Resize(cols, rows int) error {
	return pty.Setsize(c.ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func splitLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func splitPipe(s string) []string { return strings.Split(s, "|") }

// KillOrphan finds and kills orphaned tmux attach-session processes for a specific session.
// This prevents duplicate input when the server crashes and restarts.
// Matching is exact on the session name: a naive substring search for
// "-t dev" would also match "-t dev2", killing unrelated sessions.
func KillOrphan(sessionName string) {
	// List candidate attach clients with their full command lines, then
	// filter for an exact "-t <name>" argument pair.
	out, err := exec.Command("pgrep", "-af", "tmux attach-session").Output()
	if err != nil {
		return // no attach clients at all
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 1 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		if !hasExactSessionArg(fields[1:], sessionName) {
			continue
		}
		if proc, err := os.FindProcess(pid); err == nil {
			proc.Kill()
			logger.Infof("cleanup: killed orphaned attach handler pid=%d session=%s", pid, sessionName)
		}
	}
}

// hasExactSessionArg reports whether argv contains the exact pair
// ["-t" sessionName] (or the equivalent ["-t=sessionName"] / merged forms
// tmux accepts, e.g. "-tname").
func hasExactSessionArg(argv []string, sessionName string) bool {
	for i, arg := range argv {
		if arg == "-t" {
			if i+1 < len(argv) && argv[i+1] == sessionName {
				return true
			}
			continue
		}
		if rest, ok := strings.CutPrefix(arg, "-t"); ok && rest != "" {
			// Forms: -tname, -t=name. Strip a leading '=' for the latter.
			if strings.TrimPrefix(rest, "=") == sessionName {
				return true
			}
		}
	}
	return false
}

func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}
