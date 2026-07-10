package parse

import "time"

// Usage holds token counts for a single billable event.
type Usage struct {
	Input        int64 `json:"input"`
	Output       int64 `json:"output"`
	CacheRead    int64 `json:"cache_read"`
	CacheWrite5m int64 `json:"cache_write_5m"`
	CacheWrite1h int64 `json:"cache_write_1h"`
}

func (u Usage) Total() int64 {
	return u.Input + u.Output + u.CacheRead + u.CacheWrite5m + u.CacheWrite1h
}

func (u *Usage) Add(o Usage) {
	u.Input += o.Input
	u.Output += o.Output
	u.CacheRead += o.CacheRead
	u.CacheWrite5m += o.CacheWrite5m
	u.CacheWrite1h += o.CacheWrite1h
}

// Event is one deduplicated billable API interaction from an agent log.
type Event struct {
	Time      time.Time
	Source    string // "claude" | "codex"
	Model     string
	Project   string // normalized working directory
	SessionID string
	Usage     Usage
}
