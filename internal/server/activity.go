package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/aerostone/webtmux/internal/logger"
)

// ActivityType represents the type of activity
type ActivityType string

const (
	ActivityLogin      ActivityType = "login"
	ActivityLogout     ActivityType = "logout"
	ActivitySession    ActivityType = "session"
	ActivityFileUpload ActivityType = "file_upload"
	ActivityFileDelete ActivityType = "file_delete"
	ActivityMkdir      ActivityType = "mkdir"
)

// Activity represents a user activity event
type Activity struct {
	ID        string       `json:"id"`
	Type      ActivityType `json:"type"`
	Timestamp time.Time    `json:"timestamp"`
	Detail    string       `json:"detail"`
	Session   string       `json:"session,omitempty"`
	Path      string       `json:"path,omitempty"`
	Remote    string       `json:"remote,omitempty"`
}

// ActivityStore manages activity logging and retrieval
type ActivityStore struct {
	mu         sync.RWMutex
	activities []Activity
	file       string
	maxItems   int
}

// NewActivityStore creates a new activity store
func NewActivityStore(dataDir string) (*ActivityStore, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	store := &ActivityStore{
		file:     filepath.Join(dataDir, "activities.json"),
		maxItems: 100, // Keep last 100 activities
	}

	// Load existing activities
	store.load()

	return store, nil
}

// load reads activities from file
func (s *ActivityStore) load() {
	data, err := os.ReadFile(s.file)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warnf("activity: load error: %v", err)
		}
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	json.Unmarshal(data, &s.activities)
}

// save writes activities to file
func (s *ActivityStore) save() {
	s.mu.RLock()
	data, err := json.MarshalIndent(s.activities, "", "  ")
	s.mu.RUnlock()

	if err != nil {
		logger.Warnf("activity: marshal error: %v", err)
		return
	}

	if err := os.WriteFile(s.file, data, 0644); err != nil {
		logger.Warnf("activity: save error: %v", err)
	}
}

// Log adds a new activity
func (s *ActivityStore) Log(actType ActivityType, detail string, opts ...func(*Activity)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	act := Activity{
		ID:        generateID(),
		Type:      actType,
		Timestamp: time.Now(),
		Detail:    detail,
	}

	for _, opt := range opts {
		opt(&act)
	}

	s.activities = append([]Activity{act}, s.activities...)

	// Trim to max items
	if len(s.activities) > s.maxItems {
		s.activities = s.activities[:s.maxItems]
	}

	// Save asynchronously
	go s.save()
}

// WithSession sets the session field
func WithSession(session string) func(*Activity) {
	return func(a *Activity) { a.Session = session }
}

// WithPath sets the path field
func WithPath(path string) func(*Activity) {
	return func(a *Activity) { a.Path = path }
}

// WithRemote sets the remote field
func WithRemote(remote string) func(*Activity) {
	return func(a *Activity) { a.Remote = remote }
}

// GetRecent returns the most recent activities
func (s *ActivityStore) GetRecent(limit int) []Activity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.activities) {
		limit = len(s.activities)
	}

	result := make([]Activity, limit)
	copy(result, s.activities[:limit])
	return result
}

// GetByType returns activities filtered by type
func (s *ActivityStore) GetByType(actType ActivityType, limit int) []Activity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Activity
	for _, act := range s.activities {
		if act.Type == actType {
			result = append(result, act)
			if len(result) >= limit {
				break
			}
		}
	}
	return result
}

// generateID creates a simple unique ID
func generateID() string {
	return time.Now().Format("20060102150405") + "-" + randomHex(4)
}

// randomHex generates a random hex string
func randomHex(n int) string {
	const hex = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = hex[time.Now().UnixNano()%16]
		time.Sleep(1) // Ensure different values
	}
	return string(b)
}

// handleActivities API endpoint
func (s *Server) handleActivities(w http.ResponseWriter, r *http.Request) {
	limit := 50 // Default limit

	activities := s.activities.GetRecent(limit)

	// Sort by timestamp descending (most recent first)
	sort.Slice(activities, func(i, j int) bool {
		return activities[i].Timestamp.After(activities[j].Timestamp)
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(activities)
}
