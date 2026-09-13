package backend

import (
	"fmt"
	"runtime"

	"github.com/mackerelio/go-osstat/memory"
)

// Health is a live snapshot of IDE backend health for the Perf HUD overlay.
type Health struct {
	RSSMB         float64 `json:"rssMB"`
	Goroutines    int     `json:"goroutines"`
	Terminals     int     `json:"terminals"`
	Watchers      int     `json:"watchers"`
	ACPSessions   int     `json:"acpSessions"`
	EditorBuffers int     `json:"editorBuffers"`
	WorktreesSize string  `json:"worktreesSize"`
}

// HealthMetrics computes cheap per-call counters at the 1Hz poll rate the
// Perf HUD uses. RSS via go-osstat/memory (pure Go, no cgo); on failure the
// field simply stays 0 rather than blocking the rest of the snapshot.
func (a *App) HealthMetrics() Health {
	h := Health{
		Goroutines:    runtime.NumGoroutine(),
		EditorBuffers: a.buf.Count(),
	}

	a.mu.Lock()
	h.Watchers = len(a.watchers)
	a.mu.Unlock()

	a.termMu.Lock()
	h.Terminals = len(a.sessions)
	a.termMu.Unlock()

	h.ACPSessions = len(a.agents)

	if m, err := memory.Get(); err == nil {
		h.RSSMB = float64(m.Used) / (1024 * 1024)
	}
	if h.RSSMB == 0 {
		// Fallback: report Go heap so the HUD still shows something.
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		h.RSSMB = float64(ms.Sys) / (1024 * 1024)
	}
	h.WorktreesSize = fmt.Sprintf("%d projects", len(a.reg.List()))
	return h
}
