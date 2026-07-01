package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// FileEntry represents a file/directory in the listing
type FileEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"is_dir"`
	ModTime string `json:"mod_time"`
	Mode    string `json:"mode"`
}

// validatePath checks if the path is within allowed root directory
func validatePath(path string) (string, error) {
	// Get allowed root (default to home directory)
	root, err := os.UserHomeDir()
	if err != nil {
		root = "/"
	}
	
	// Clean the path
	cleaned := filepath.Clean(path)
	
	// Check if path is within root
	if !strings.HasPrefix(cleaned, root) && cleaned != root {
		return "", fmt.Errorf("access denied: path outside allowed directory")
	}
	
	return cleaned, nil
}

func (s *Server) handleFileManager(w http.ResponseWriter, r *http.Request) {
	action := r.URL.Query().Get("action")

	switch action {
	case "list":
		s.handleFileList(w, r)
	case "upload":
		s.handleFileUpload(w, r)
	case "download":
		s.handleFileDownload(w, r)
	case "delete":
		s.handleFileDelete(w, r)
	case "mkdir":
		s.handleMkdir(w, r)
	default:
		http.Error(w, `{"error":"unknown action, use: list, upload, download, delete, mkdir"}`, http.StatusBadRequest)
	}
}

func (s *Server) handleFileList(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("path")
	if dir == "" {
		dir = "/"
	}

	// Resolve home directory
	if strings.HasPrefix(dir, "~") {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, dir[1:])
	}

	// Validate path - prevent path traversal
	var err error
	dir, err = validatePath(dir)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	result := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		result = append(result, FileEntry{
			Name:    e.Name(),
			Path:    filepath.Join(dir, e.Name()),
			Size:    info.Size(),
			IsDir:   e.IsDir(),
			ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
			Mode:    info.Mode().String(),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"path":    dir,
		"entries": result,
	})
}

func (s *Server) handleFileUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form (max 100MB)
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "parse form: " + err.Error()})
		return
	}

	destDir := r.FormValue("path")
	if destDir == "" {
		destDir, _ = os.UserHomeDir()
	}

	// Resolve home directory
	if strings.HasPrefix(destDir, "~") {
		home, _ := os.UserHomeDir()
		destDir = filepath.Join(home, destDir[1:])
	}

	// Validate path - prevent path traversal
	var err error
	destDir, err = validatePath(destDir)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}

	// Get uploaded files
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no files provided"})
		return
	}

	uploaded := make([]string, 0, len(files))
	for _, fh := range files {
		src, err := fh.Open()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "open file: " + err.Error()})
			return
		}
		defer src.Close()

		// Sanitize filename - prevent path traversal
		filename := filepath.Base(fh.Filename)
		destPath := filepath.Join(destDir, filename)

		dst, err := os.Create(destPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create file: " + err.Error()})
			return
		}
		defer dst.Close()

		written, err := io.Copy(dst, src)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "write file: " + err.Error()})
			return
		}

		log.Printf("file upload: %s (%d bytes) -> %s", filename, written, destPath)
		uploaded = append(uploaded, filename)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":   "ok",
		"uploaded": uploaded,
		"path":     destDir,
	})
}

func (s *Server) handleFileDownload(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, `{"error":"path required"}`, http.StatusBadRequest)
		return
	}

	// Resolve home directory
	if strings.HasPrefix(filePath, "~") {
		home, _ := os.UserHomeDir()
		filePath = filepath.Join(home, filePath[1:])
	}

	// Validate path - prevent path traversal
	var err error
	filePath, err = validatePath(filePath)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}

	info, err := os.Stat(filePath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}
	if info.IsDir() {
		http.Error(w, `{"error":"cannot download directory"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(filePath)))
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, filePath)
}

func (s *Server) handleFileDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path required"})
		return
	}

	// Resolve home directory
	if strings.HasPrefix(req.Path, "~") {
		home, _ := os.UserHomeDir()
		req.Path = filepath.Join(home, req.Path[1:])
	}

	// Validate path - prevent path traversal
	var err error
	req.Path, err = validatePath(req.Path)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}

	if err := os.Remove(req.Path); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	log.Printf("file delete: %s", req.Path)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleMkdir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path required"})
		return
	}

	// Resolve home directory
	if strings.HasPrefix(req.Path, "~") {
		home, _ := os.UserHomeDir()
		req.Path = filepath.Join(home, req.Path[1:])
	}

	// Validate path - prevent path traversal
	var err error
	req.Path, err = validatePath(req.Path)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}

	if err := os.MkdirAll(req.Path, 0755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	log.Printf("mkdir: %s", req.Path)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "path": req.Path})
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
