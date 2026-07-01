package tmux

import "testing"

func TestParseSessions(t *testing.T) {
	raw := "main|1|3\nwork|0|2\n"
	sessions := parseSessions(raw)

	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	if sessions[0].Name != "main" {
		t.Errorf("expected 'main', got %q", sessions[0].Name)
	}
	if sessions[0].Attached != 1 {
		t.Errorf("expected attached=1, got %d", sessions[0].Attached)
	}
	if sessions[1].Windows != 2 {
		t.Errorf("expected windows=2, got %d", sessions[1].Windows)
	}
}

func TestParseSessionsEmpty(t *testing.T) {
	sessions := parseSessions("")
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}
