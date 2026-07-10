package parse

import (
	"os"
	"path/filepath"
	"testing"
)

// Streamed Claude responses append the same message id repeatedly with
// cumulative usage; only the final occurrence may be counted.
func TestClaudeDedupesStreamedMessages(t *testing.T) {
	dir := t.TempDir()
	lines := `{"type":"assistant","timestamp":"2026-07-01T10:00:00Z","cwd":"/tmp/proj","sessionId":"s1","requestId":"req1","message":{"id":"msg1","model":"claude-opus-4-8","usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}
{"type":"assistant","timestamp":"2026-07-01T10:00:01Z","cwd":"/tmp/proj","sessionId":"s1","requestId":"req1","message":{"id":"msg1","model":"claude-opus-4-8","usage":{"input_tokens":10,"output_tokens":50,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}
{"type":"assistant","timestamp":"2026-07-01T10:01:00Z","cwd":"/tmp/proj","sessionId":"s1","requestId":"req2","message":{"id":"msg2","model":"claude-opus-4-8","usage":{"input_tokens":20,"output_tokens":7,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}
`
	writeFile(t, filepath.Join(dir, "s1.jsonl"), lines)

	events, err := ParseClaude(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("want 2 deduplicated events, got %d", len(events))
	}
	var output int64
	for _, e := range events {
		output += e.Usage.Output
	}
	if output != 57 { // 50 (final msg1) + 7 (msg2), not 5+50+7
		t.Fatalf("want output 57 after dedupe, got %d", output)
	}
}

// Codex token_count events carry cumulative totals; events contribute deltas,
// and duplicate/repeated events must not double-count.
func TestCodexUsesCumulativeDeltas(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "2026", "07", "01")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lines := `{"timestamp":"2026-07-01T10:00:00Z","type":"turn_context","payload":{"model":"gpt-5.4","cwd":"/tmp/proj"}}
{"timestamp":"2026-07-01T10:00:10Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":10,"total_tokens":110}}}}
{"timestamp":"2026-07-01T10:00:11Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":10,"total_tokens":110}}}}
{"timestamp":"2026-07-01T10:00:20Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":250,"cached_input_tokens":120,"output_tokens":30,"total_tokens":280}}}}
`
	writeFile(t, filepath.Join(dir, "rollout-x.jsonl"), lines)

	events, err := ParseCodex(filepath.Dir(filepath.Dir(filepath.Dir(dir))))
	if err != nil {
		t.Fatal(err)
	}
	var total Usage
	for _, e := range events {
		if e.Model != "gpt-5.4" {
			t.Fatalf("model not attributed from turn_context: %q", e.Model)
		}
		total.Add(e.Usage)
	}
	// Must equal the final cumulative counters, not the sum of repeats.
	if total.Input != 130 || total.CacheRead != 120 || total.Output != 30 {
		t.Fatalf("delta accounting wrong: %+v", total)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
