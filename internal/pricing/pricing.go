// Package pricing converts token usage into API-equivalent dollar cost.
//
// Rates are per million tokens. Cache rates derive from the documented
// multipliers: read = 0.1x input, 5m write = 1.25x input, 1h write = 2x input.
// Table as of 2026-07 (platform.claude.com/docs/en/pricing). Override or
// extend via ~/.config/burned/pricing.json: {"model-prefix": {"input": 5, "output": 25}}.
package pricing

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/phrypy/burned/internal/parse"
)

type Rate struct {
	Input  float64 `json:"input"`
	Output float64 `json:"output"`
}

// Longest matching prefix wins, so more specific entries need no ordering.
var defaultRates = map[string]Rate{
	"claude-fable-5":    {10, 50},
	"claude-mythos-5":   {10, 50},
	"claude-opus-4":     {5, 25},  // 4.5 through 4.8
	"claude-opus-4-1":   {15, 75}, // Opus 4.1: legacy pricing
	"claude-opus-4-0":   {15, 75},
	"claude-opus-4-2":   {15, 75}, // catches Opus 4.0's dated id claude-opus-4-20250514
	"claude-sonnet-5":   {3, 15},
	"claude-sonnet-4":   {3, 15},
	"claude-3-7-sonnet": {3, 15},
	"claude-3-5-sonnet": {3, 15},
	"claude-haiku-4-5":  {1, 5},
	"claude-3-5-haiku":  {0.8, 4},
	"claude-3-haiku":    {0.25, 1.25},
	// OpenAI (Codex): cached reads are 0.1x input, same as Claude, so the
	// shared formula applies. Codex logs report no cache-write tokens.
	"gpt-5.5": {5, 30},
	"gpt-5.4": {2.5, 15},
}

type Table struct {
	rates   map[string]Rate
	Unknown map[string]bool // models seen without a rate
}

// Load returns the built-in table merged with the user override file, if any.
func Load() *Table {
	rates := make(map[string]Rate, len(defaultRates))
	for k, v := range defaultRates {
		rates[k] = v
	}
	if home, err := os.UserHomeDir(); err == nil {
		if b, err := os.ReadFile(filepath.Join(home, ".config", "burned", "pricing.json")); err == nil {
			var user map[string]Rate
			if json.Unmarshal(b, &user) == nil {
				for k, v := range user {
					rates[k] = v
				}
			}
		}
	}
	return &Table{rates: rates, Unknown: make(map[string]bool)}
}

// Cost returns the API-equivalent dollar cost of usage on model, and whether
// the model was priced. Unpriced models are recorded in t.Unknown.
func (t *Table) Cost(model string, u parse.Usage) (float64, bool) {
	rate, ok := t.lookup(model)
	if !ok {
		t.Unknown[model] = true
		return 0, false
	}
	const m = 1e6
	in := rate.Input
	return float64(u.Input)/m*in +
		float64(u.Output)/m*rate.Output +
		float64(u.CacheRead)/m*in*0.1 +
		float64(u.CacheWrite5m)/m*in*1.25 +
		float64(u.CacheWrite1h)/m*in*2, true
}

// NoCacheCost returns what the same usage would cost if nothing were cached:
// every cache-read and cache-write token billed as a plain input token.
func (t *Table) NoCacheCost(model string, u parse.Usage) (float64, bool) {
	rate, ok := t.lookup(model)
	if !ok {
		return 0, false
	}
	const m = 1e6
	inputAll := u.Input + u.CacheRead + u.CacheWrite5m + u.CacheWrite1h
	return float64(inputAll)/m*rate.Input + float64(u.Output)/m*rate.Output, true
}

func (t *Table) lookup(model string) (Rate, bool) {
	best, found := "", false
	var rate Rate
	for prefix, r := range t.rates {
		if strings.HasPrefix(model, prefix) && len(prefix) > len(best) {
			best, rate, found = prefix, r, true
		}
	}
	return rate, found
}
