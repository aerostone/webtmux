package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Level represents log severity
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

var levelNames = map[Level]string{
	LevelDebug: "DEBUG",
	LevelInfo:  "INFO",
	LevelWarn:  "WARN",
	LevelError: "ERROR",
}

var currentLevel = LevelInfo
var mu sync.Mutex
var out io.Writer = os.Stderr
var logDir string
var maxAgeDays = 3

// SetLevel sets the minimum log level
func SetLevel(l Level) {
	mu.Lock()
	defer mu.Unlock()
	currentLevel = l
}

// ParseLevel parses a string level
func ParseLevel(s string) Level {
	switch strings.ToUpper(s) {
	case "DEBUG":
		return LevelDebug
	case "INFO":
		return LevelInfo
	case "WARN", "WARNING":
		return LevelWarn
	case "ERROR":
		return LevelError
	default:
		return LevelInfo
	}
}

// Init initializes the logger with file output and rotation
func Init(dir string, level Level, ageDays int) error {
	mu.Lock()
	defer mu.Unlock()

	if ageDays > 0 {
		maxAgeDays = ageDays
	}

	if dir == "" {
		logDir = filepath.Join(os.TempDir(), "webtmux-logs")
	} else {
		logDir = dir
	}

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	currentLevel = level
	out = io.MultiWriter(os.Stderr, &rotatingWriter{})

	// Set standard log output
	log.SetOutput(out)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	// Start cleanup goroutine
	go cleanupLoop()

	return nil
}

// rotatingWriter writes to daily log files
type rotatingWriter struct{}

func (w *rotatingWriter) Write(p []byte) (n int, err error) {
	mu.Lock()
	defer mu.Unlock()

	filename := filepath.Join(logDir, time.Now().Format("2006-01-02")+".log")
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	return f.Write(p)
}

// cleanupLoop removes old log files
func cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		cleanupOldLogs()
	}
}

func cleanupOldLogs() {
	mu.Lock()
	defer mu.Unlock()

	entries, err := os.ReadDir(logDir)
	if err != nil {
		return
	}

	cutoff := time.Now().AddDate(0, 0, -maxAgeDays)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".log") {
			continue
		}
		dateStr := strings.TrimSuffix(name, ".log")
		t, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}
		if t.Before(cutoff) {
			os.Remove(filepath.Join(logDir, name))
		}
	}
}

// CleanupNow forces immediate cleanup of old logs
func CleanupNow() {
	cleanupOldLogs()
}

// ListLogFiles returns available log files
func ListLogFiles() ([]string, error) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".log") {
			files = append(files, filepath.Join(logDir, e.Name()))
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	return files, nil
}

// LogDir returns the current log directory
func LogDir() string {
	return logDir
}

// --- Logging functions ---

func logf(level Level, format string, args ...interface{}) {
	mu.Lock()
	l := currentLevel
	w := out
	mu.Unlock()

	if level < l {
		return
	}

	msg := fmt.Sprintf(format, args...)
	ts := time.Now().Format("2006-01-02 15:04:05.000")
	fmt.Fprintf(w, "%s [%s] %s\n", ts, levelNames[level], msg)
}

func Debugf(format string, args ...interface{}) {
	logf(LevelDebug, format, args...)
}

func Infof(format string, args ...interface{}) {
	logf(LevelInfo, format, args...)
}

func Warnf(format string, args ...interface{}) {
	logf(LevelWarn, format, args...)
}

func Errorf(format string, args ...interface{}) {
	logf(LevelError, format, args...)
}

func Fatalf(format string, args ...interface{}) {
	logf(LevelError, format, args...)
	os.Exit(1)
}
