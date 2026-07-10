// burned — see where your AI coding-agent tokens went.
//
// Reads local session logs (Claude Code, Codex CLI), never makes a network
// call, and prints an API-equivalent cost X-ray: per model, per project,
// most expensive sessions, cache savings, daily burn.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/phrypy/burned/internal/parse"
	"github.com/phrypy/burned/internal/pricing"
	"github.com/phrypy/burned/internal/report"
)

func main() {
	since := flag.String("since", "", "only include usage newer than this, e.g. 7d, 24h, 30d (default: everything)")
	source := flag.String("source", "all", "which logs to scan: claude, codex, all")
	project := flag.String("project", "", "filter to projects whose path contains this substring")
	top := flag.Int("top", 10, "rows per table")
	asJSON := flag.Bool("json", false, "emit aggregated report as JSON")
	flag.Parse()

	var events []parse.Event
	if *source == "claude" || *source == "all" {
		es, err := parse.ParseClaude(parse.ClaudeDir())
		if err != nil && *source == "claude" {
			fatal("scanning Claude Code logs: %v", err)
		}
		events = append(events, es...)
	}
	if *source == "codex" || *source == "all" {
		es, err := parse.ParseCodex(parse.CodexDir())
		if err != nil && *source == "codex" {
			fatal("scanning Codex logs: %v", err)
		}
		events = append(events, es...)
	}

	if *since != "" {
		cutoff := time.Now().Add(-parseDuration(*since))
		events = filter(events, func(e parse.Event) bool { return e.Time.After(cutoff) })
	}
	if *project != "" {
		needle := strings.ToLower(*project)
		events = filter(events, func(e parse.Event) bool {
			return strings.Contains(strings.ToLower(e.Project), needle)
		})
	}
	if len(events) == 0 {
		fmt.Println("no agent usage found — nothing to burn-report")
		return
	}

	r := report.Build(events, pricing.Load())
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(r); err != nil {
			fatal("encoding JSON: %v", err)
		}
		return
	}
	r.Render(os.Stdout, *top)
}

var durRe = regexp.MustCompile(`^(\d+)([dhm])$`)

// parseDuration accepts 7d / 24h / 30m style durations.
func parseDuration(s string) time.Duration {
	m := durRe.FindStringSubmatch(s)
	if m == nil {
		fatal("invalid --since value %q (use e.g. 7d, 24h, 30m)", s)
	}
	n, _ := strconv.Atoi(m[1])
	switch m[2] {
	case "d":
		return time.Duration(n) * 24 * time.Hour
	case "h":
		return time.Duration(n) * time.Hour
	default:
		return time.Duration(n) * time.Minute
	}
}

func filter(events []parse.Event, keep func(parse.Event) bool) []parse.Event {
	out := events[:0]
	for _, e := range events {
		if keep(e) {
			out = append(out, e)
		}
	}
	return out
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "burned: "+format+"\n", args...)
	os.Exit(1)
}
