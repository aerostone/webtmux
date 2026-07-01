package tmux

import (
	"testing"
)

func TestListSessions(t *testing.T) {
	mgr := NewManager("")
	sessions, err := mgr.ListSessions()
	if err != nil {
		t.Skipf("tmux not available: %v", err)
	}
	if sessions == nil {
		sessions = []Session{}
	}
	t.Logf("found %d sessions", len(sessions))
}

func TestCreateAndKillSession(t *testing.T) {
	mgr := NewManager("")
	name := "webtmux-test-create-kill"

	err := mgr.CreateSession(name)
	if err != nil {
		t.Skipf("tmux not available: %v", err)
	}
	defer mgr.KillSession(name)

	sessions, err := mgr.ListSessions()
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	found := false
	for _, s := range sessions {
		if s.Name == name {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("session %q not found after create", name)
	}
}

func TestKillNonexistentSession(t *testing.T) {
	mgr := NewManager("")
	err := mgr.KillSession("webtmux-does-not-exist-xyz")
	if err == nil {
		t.Error("expected error killing nonexistent session")
	}
}

func TestAttachSession(t *testing.T) {
	mgr := NewManager("")
	name := "webtmux-test-attach"

	if err := mgr.CreateSession(name); err != nil {
		t.Skipf("tmux not available: %v", err)
	}
	defer mgr.KillSession(name)

	conn, err := mgr.AttachSession(name)
	if err != nil {
		t.Fatalf("attach failed: %v", err)
	}
	defer conn.Close()

	t.Logf("attached to session %q", name)
}
