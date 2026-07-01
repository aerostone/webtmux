package server

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
)

// SystemInfo holds system metrics
type SystemInfo struct {
	CPU    string      `json:"cpu"`
	Memory MemoryInfo  `json:"memory"`
	Disk   DiskInfo    `json:"disk"`
	Load   []string    `json:"load"`
	Uptime string      `json:"uptime"`
	Procs  string      `json:"procs"`
}

type MemoryInfo struct {
	Used    string `json:"used"`
	Total   string `json:"total"`
	Percent string `json:"percent"`
}

type DiskInfo struct {
	Used    string `json:"used"`
	Total   string `json:"total"`
	Percent string `json:"percent"`
}

func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	info := SystemInfo{
		CPU:    getCPUUsage(),
		Memory: getMemoryInfo(),
		Disk:   getDiskInfo(),
		Load:   getLoadAvg(),
		Uptime: getUptime(),
		Procs:  getProcessCount(),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func getCPUUsage() string {
	if runtime.GOOS == "linux" {
		out, err := exec.Command("sh", "-c", `top -bn1 | grep "Cpu(s)" | awk '{print $2}'`).Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return "0"
}

func getMemoryInfo() MemoryInfo {
	if runtime.GOOS == "linux" {
		out, err := exec.Command("sh", "-c", `free -h | awk '/Mem/{print $3"/"$2}'`).Output()
		if err == nil {
			parts := strings.Split(strings.TrimSpace(string(out)), "/")
			if len(parts) == 2 {
				return MemoryInfo{
					Used:  parts[0],
					Total: parts[1],
				}
			}
		}
		// Fallback to /proc/meminfo
		out, err = exec.Command("sh", "-c", `free | awk '/Mem/{printf "%.1f", $3/$2*100}'`).Output()
		if err == nil {
			return MemoryInfo{
				Percent: strings.TrimSpace(string(out)),
			}
		}
	}
	return MemoryInfo{}
}

func getDiskInfo() DiskInfo {
	if runtime.GOOS == "linux" {
		out, err := exec.Command("sh", "-c", `df -h / | awk 'NR==2{print $3"/"$2}'`).Output()
		if err == nil {
			parts := strings.Split(strings.TrimSpace(string(out)), "/")
			if len(parts) == 2 {
				return DiskInfo{
					Used:  parts[0],
					Total: parts[1],
				}
			}
		}
	}
	return DiskInfo{}
}

func getLoadAvg() []string {
	if runtime.GOOS == "linux" {
		out, err := exec.Command("sh", "-c", `cat /proc/loadavg | awk '{print $1, $2, $3}'`).Output()
		if err == nil {
			return strings.Fields(strings.TrimSpace(string(out)))
		}
	}
	return []string{"0", "0", "0"}
}

func getUptime() string {
	if runtime.GOOS == "linux" {
		out, err := exec.Command("sh", "-c", `uptime -p`).Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
		// Fallback to uptime command
		out, err = exec.Command("sh", "-c", `uptime | awk -F'up' '{print $2}' | awk -F',' '{print $1}'`).Output()
		if err == nil {
			return "up " + strings.TrimSpace(string(out))
		}
	}
	return "unknown"
}

func getProcessCount() string {
	if runtime.GOOS == "linux" {
		out, err := exec.Command("sh", "-c", `ps aux | wc -l`).Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return "0"
}