/**
 * Live activity handler for 9router-go desktop widget.
 * 
 * Mirip struktur useLiveActivity.ts dari jrouters, tetapi diimplementasi
 * di sisi Go sebagai endpoint HTTP GET /api/live.
 * 
 * Menggunakan mutex satu arah (one mutex, no DB) seperti gateway di jrouters,
 * payload few ratus bytes, polling 1200ms per interval untuk seluruh app.
 */

package handlers

import (
	"encoding/json"
	"net/http"
	"sync"
)

// LiveSnapshot adalah struktur data live activity yang dikirim ke klien.
// Disesuaikan dengan antarmuka Svelte LiveActivityWidget.
// Field ini sesuai dengan payload JSON dari server 9router-go (/api/live).
type LiveSnapshot struct {
	GeneratedAt string  `json:"generatedAt"`
	Rps         float64 `json:"rps"`
	Tps         float64 `json:"tps"`
	Active      int     `json:"active"`
	Waiting     int     `json:"waiting"`
	Inflight    int     `json:"inflight"`
	Streams     int     `json:"streams"`
	Errors      int     `json:"errors"`
	Retries     int     `json:"retries"`
	OldestAgeSec int    `json:"oldestAgeSec"`
	Count       int     `json:"count"`
	// Fields for sessions list (widget hanya menampilkan beberapa)
	Sessions []struct {
		Model    string `json:"model"`
		Combo    string `json:"combo"`
		Provider string `json:"provider"`
		Step     string `json:"step"`
		Status   string `json:"status"`
		Finished bool   `json:"finished"`
		TtfbMs   int64  `json:"ttfbMs"`
		Elapsed  int64  `json:"elapsedMs"`
	} `json:"sessions"`
}

// liveActivityState menyimpan state live activity di memory.
// Dipastikan akses thread-safe via mu.
type liveActivityState struct {
	mu sync.Mutex
	snapshot LiveSnapshot
}

var liveState = liveActivityState{
	snapshot: LiveSnapshot{
		GeneratedAt: "",
		Rps:         0,
		Tps:         0,
		Active:      0,
		Waiting:     0,
		Inflight:    0,
		Streams:     0,
		Errors:      0,
		Retries:     0,
		OldestAgeSec: 0,
		Count:       0,
	},
}

// getLiveSnapshot mengembalikan salinan tercurrent state live activity.
// Dipanggil dari handler HTTP.
func getLiveSnapshot() LiveSnapshot {
	liveState.mu.Lock()
	defer liveState.mu.Unlock()
	return liveState.snapshot
}

// UpdateLiveSnapshot memperbarui state live activity (dipanggil dari daemon).
// Terexport agar bisa dipanggil dari handler chat (internal/handlers/chat).
func UpdateLiveSnapshot(s LiveSnapshot) {
	liveState.mu.Lock()
	defer liveState.mu.Unlock()
	liveState.snapshot = s
}

// HandleLiveActivity menangani request GET /api/live.
func HandleLiveActivity(w http.ResponseWriter, r *http.Request) {
	snapshot := getLiveSnapshot()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snapshot)
}