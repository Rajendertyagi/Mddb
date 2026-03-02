package main

import (
	"net/http"
	"os"
	"runtime"
	"time"

	json "github.com/goccy/go-json"
)

// Global variable to track server start time
var serverStartTime = time.Now()

// ---- Request/Response types ----

type SystemInfoResponse struct {
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	NumCPU        int    `json:"numCPU"`
	GoVersion     string `json:"goVersion"`
	Version       string `json:"version"`
	UptimeSeconds int64  `json:"uptimeSeconds"`
	MemoryTotal   uint64 `json:"memoryTotal"`
	MemoryUsed    uint64 `json:"memoryUsed"`
	MemorySystem  uint64 `json:"memorySystem"`
	MemoryHeap    uint64 `json:"memoryHeap"`
	NumGoroutines int    `json:"numGoroutines"`
}

// ---- Handlers ----

// handleSystemInfo returns system information
func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get hostname
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// Get memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Calculate uptime
	uptime := int64(time.Since(serverStartTime).Seconds())

	response := SystemInfoResponse{
		Hostname:      hostname,
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		NumCPU:        runtime.NumCPU(),
		GoVersion:     runtime.Version(),
		Version:       VERSION,
		UptimeSeconds: uptime,
		MemoryTotal:   memStats.TotalAlloc,
		MemoryUsed:    memStats.Alloc,
		MemorySystem:  memStats.Sys,
		MemoryHeap:    memStats.HeapInuse,
		NumGoroutines: runtime.NumGoroutine(),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, `{"error":"failed to encode response"}`, http.StatusInternalServerError)
	}
}
