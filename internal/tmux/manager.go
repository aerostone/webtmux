package tmux

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/creack/pty"
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
type Resizable interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
}

func (m *Manager) AttachSession(name string) (Resizable, error) {
	args := m.tmuxArgs("attach-session", "-t", name)
	cmd := exec.Command("tmux", args...)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("tmux attach %q: %w", name, err)
	}
	return &ptyConn{ptmx: ptmx, cmd: cmd}, nil
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
}

func (c *ptyConn) Read(p []byte) (int, error)  { return c.ptmx.Read(p) }
func (c *ptyConn) Write(p []byte) (int, error) { return c.ptmx.Write(p) }
func (c *ptyConn) Close() error {
	c.ptmx.Close()
	if c.cmd.Process != nil {
		c.cmd.Process.Kill()
	}
	return nil
}

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

func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}
