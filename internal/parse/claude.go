package parse

import (
	"bufio"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// claudeLine is the subset of a Claude Code session JSONL entry we need.
type claudeLine struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	CWD       string `json:"cwd"`
	SessionID string `json:"sessionId"`
	RequestID string `json:"requestId"`
	Message   struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage *struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			CacheCreation            *struct {
				Ephemeral5m int64 `json:"ephemeral_5m_input_tokens"`
				Ephemeral1h int64 `json:"ephemeral_1h_input_tokens"`
			} `json:"cache_creation"`
		} `json:"usage"`
	} `json:"message"`
}

// ClaudeDir returns the default Claude Code projects directory.
func ClaudeDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects")
}

// ParseClaude scans every session file under dir and returns deduplicated events.
// Streamed responses append the same message repeatedly with cumulative usage,
// so only the last occurrence of each (message.id, requestId) pair is kept.
func ParseClaude(dir string) ([]Event, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entry: skip, don't abort the scan
		}
		if !d.IsDir() && strings.HasSuffix(path, ".jsonl") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	latest := make(map[string]Event) // dedupe key -> last-seen event
	var undeduped []Event            // entries without a usable dedupe key
	for _, f := range files {
		parseClaudeFile(f, latest, &undeduped)
	}

	events := make([]Event, 0, len(latest)+len(undeduped))
	for _, e := range latest {
		events = append(events, e)
	}
	return append(events, undeduped...), nil
}

func parseClaudeFile(path string, latest map[string]Event, undeduped *[]Event) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	r := bufio.NewReaderSize(f, 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			if e, key, ok := claudeEvent(line); ok {
				if key == "" {
					*undeduped = append(*undeduped, e)
				} else {
					latest[key] = e
				}
			}
		}
		if err != nil {
			if err != io.EOF {
				return
			}
			break
		}
	}
}

func claudeEvent(line []byte) (Event, string, bool) {
	var l claudeLine
	if json.Unmarshal(line, &l) != nil {
		return Event{}, "", false
	}
	if l.Type != "assistant" || l.Message.Usage == nil {
		return Event{}, "", false
	}
	if l.Message.Model == "" || l.Message.Model == "<synthetic>" {
		return Event{}, "", false
	}

	u := l.Message.Usage
	usage := Usage{
		Input:     u.InputTokens,
		Output:    u.OutputTokens,
		CacheRead: u.CacheReadInputTokens,
	}
	if cc := u.CacheCreation; cc != nil {
		usage.CacheWrite5m = cc.Ephemeral5m
		usage.CacheWrite1h = cc.Ephemeral1h
	} else {
		usage.CacheWrite5m = u.CacheCreationInputTokens
	}
	if usage.Total() == 0 {
		return Event{}, "", false
	}

	t, _ := time.Parse(time.RFC3339, l.Timestamp)
	e := Event{
		Time:      t,
		Source:    "claude",
		Model:     l.Message.Model,
		Project:   normalizeProject(l.CWD),
		SessionID: l.SessionID,
		Usage:     usage,
	}
	var key string
	if l.Message.ID != "" {
		key = l.Message.ID + ":" + l.RequestID
	}
	return e, key, true
}

// normalizeProject folds Claude worktree checkouts back into their parent
// project and trims the path to something readable.
func normalizeProject(cwd string) string {
	if cwd == "" {
		return "(unknown)"
	}
	if i := strings.Index(cwd, "/.claude/worktrees/"); i >= 0 {
		cwd = cwd[:i]
	}
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(cwd, home) {
		cwd = "~" + strings.TrimPrefix(cwd, home)
	}
	return cwd
}
