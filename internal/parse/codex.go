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

// codexLine covers the three Codex rollout event shapes we need. Note the
// asymmetry: session_meta and turn_context are top-level `type`s whose payload
// carries the fields directly, while token_count arrives nested inside an
// event_msg payload.
type codexLine struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Payload   struct {
		Type  string `json:"type"`
		Model string `json:"model"`
		CWD   string `json:"cwd"`
		Info  *struct {
			TotalTokenUsage *codexUsage `json:"total_token_usage"`
		} `json:"info"`
	} `json:"payload"`
}

type codexUsage struct {
	InputTokens       int64 `json:"input_tokens"`
	CachedInputTokens int64 `json:"cached_input_tokens"`
	OutputTokens      int64 `json:"output_tokens"`
	TotalTokens       int64 `json:"total_tokens"`
}

// CodexDir returns the default Codex CLI sessions directory.
func CodexDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex", "sessions")
}

// ParseCodex scans Codex rollout files. token_count events carry a cumulative
// total_token_usage for the session; each event contributes the delta since
// the previous one (summing last_token_usage instead double-counts repeated
// events). Usage is attributed to the model/cwd of the most recent
// session_meta / turn_context.
func ParseCodex(dir string) ([]Event, error) {
	var events []Event
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && strings.HasSuffix(path, ".jsonl") {
			events = append(events, parseCodexFile(path)...)
		}
		return nil
	})
	return events, err
}

func parseCodexFile(path string) []Event {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	session := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	model, project := "(unknown)", "(unknown)"
	var prev codexUsage
	var events []Event

	r := bufio.NewReaderSize(f, 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			var l codexLine
			if json.Unmarshal(line, &l) == nil {
				switch {
				case l.Type == "turn_context" || l.Type == "session_meta":
					if l.Payload.Model != "" {
						model = l.Payload.Model
					}
					if l.Payload.CWD != "" {
						project = normalizeProject(l.Payload.CWD)
					}
				case l.Payload.Type == "token_count":
					if info := l.Payload.Info; info != nil && info.TotalTokenUsage != nil {
						cur := *info.TotalTokenUsage
						if cur.TotalTokens < prev.TotalTokens {
							prev = codexUsage{} // counter reset (fork/compaction)
						}
						usage := Usage{
							// input_tokens includes the cached portion; split it out
							Input:     clampPos(cur.InputTokens - cur.CachedInputTokens - (prev.InputTokens - prev.CachedInputTokens)),
							CacheRead: clampPos(cur.CachedInputTokens - prev.CachedInputTokens),
							Output:    clampPos(cur.OutputTokens - prev.OutputTokens),
						}
						prev = cur
						if usage.Total() == 0 {
							break
						}
						t, _ := time.Parse(time.RFC3339, l.Timestamp)
						events = append(events, Event{
							Time:      t,
							Source:    "codex",
							Model:     model,
							Project:   project,
							SessionID: session,
							Usage:     usage,
						})
					}
				}
			}
		}
		if err != nil {
			if err != io.EOF {
				return events
			}
			break
		}
	}
	return events
}

func clampPos(n int64) int64 {
	if n < 0 {
		return 0
	}
	return n
}
