package tmux

type Session struct {
	Name     string `json:"name"`
	Attached int    `json:"attached"`
	Windows  int    `json:"windows"`
}

func parseSessions(raw string) []Session {
	var sessions []Session
	for _, line := range splitLines(raw) {
		parts := splitPipe(line)
		if len(parts) < 3 {
			continue
		}
		sessions = append(sessions, Session{
			Name:     parts[0],
			Attached: parseInt(parts[1]),
			Windows:  parseInt(parts[2]),
		})
	}
	return sessions
}
