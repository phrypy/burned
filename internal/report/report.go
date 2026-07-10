// Package report aggregates parsed events and renders the terminal X-ray.
package report

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/phrypy/burned/internal/parse"
	"github.com/phrypy/burned/internal/pricing"
)

type bucket struct {
	Usage parse.Usage
	Cost  float64
	Count int
}

type Report struct {
	Total       bucket
	NoCacheCost float64
	First, Last time.Time
	ByModel     map[string]*bucket
	ByProject   map[string]*bucket
	BySession   map[string]*bucket
	SessionMeta map[string]sessionMeta
	ByDay       map[string]*bucket // local date, YYYY-MM-DD
	Unknown     map[string]bool
	Sessions    int
}

type sessionMeta struct {
	Project string
	Last    time.Time
}

func Build(events []parse.Event, table *pricing.Table) *Report {
	r := &Report{
		ByModel:     map[string]*bucket{},
		ByProject:   map[string]*bucket{},
		BySession:   map[string]*bucket{},
		SessionMeta: map[string]sessionMeta{},
		ByDay:       map[string]*bucket{},
	}
	for _, e := range events {
		cost, priced := table.Cost(e.Model, e.Usage)
		if noCache, ok := table.NoCacheCost(e.Model, e.Usage); ok {
			r.NoCacheCost += noCache
		}
		add := func(m map[string]*bucket, key string) {
			b := m[key]
			if b == nil {
				b = &bucket{}
				m[key] = b
			}
			b.Usage.Add(e.Usage)
			b.Cost += cost
			b.Count++
		}
		r.Total.Usage.Add(e.Usage)
		r.Total.Count++
		if priced {
			r.Total.Cost += cost
		}
		add(r.ByModel, e.Model)
		add(r.ByProject, e.Project)
		add(r.BySession, e.SessionID)
		if !e.Time.IsZero() {
			add(r.ByDay, e.Time.Local().Format("2006-01-02"))
			if r.First.IsZero() || e.Time.Before(r.First) {
				r.First = e.Time
			}
			if e.Time.After(r.Last) {
				r.Last = e.Time
			}
		}
		if meta, ok := r.SessionMeta[e.SessionID]; !ok || e.Time.After(meta.Last) {
			r.SessionMeta[e.SessionID] = sessionMeta{Project: e.Project, Last: e.Time}
		}
	}
	r.Sessions = len(r.BySession)
	r.Unknown = table.Unknown
	return r
}

// Render writes the human-readable report.
func (r *Report) Render(w io.Writer, top int) {
	saved := r.NoCacheCost - r.Total.Cost

	fmt.Fprintf(w, "\n  burned — where your tokens went\n")
	if !r.First.IsZero() {
		fmt.Fprintf(w, "  %s → %s\n", r.First.Local().Format("2 Jan 2006"), r.Last.Local().Format("2 Jan 2006"))
	}
	fmt.Fprintf(w, "\n  API-equivalent value   %s\n", dollars(r.Total.Cost))
	fmt.Fprintf(w, "  Total tokens           %s across %d sessions (%d requests)\n",
		humanTokens(r.Total.Usage.Total()), r.Sessions, r.Total.Count)
	if saved > 0 {
		fmt.Fprintf(w, "  Saved by caching       %s (would be %s uncached)\n", dollars(saved), dollars(r.NoCacheCost))
	}

	fmt.Fprintf(w, "\n  BY MODEL\n")
	renderTable(w, r.ByModel, top, nil)

	fmt.Fprintf(w, "\n  BY PROJECT\n")
	renderTable(w, r.ByProject, top, nil)

	fmt.Fprintf(w, "\n  MOST EXPENSIVE SESSIONS\n")
	renderTable(w, r.BySession, 5, func(key string) string {
		meta := r.SessionMeta[key]
		id := key
		if len(id) > 8 {
			id = id[:8]
		}
		when := ""
		if !meta.Last.IsZero() {
			when = meta.Last.Local().Format("2 Jan")
		}
		return fmt.Sprintf("%s  %s (%s)", id, meta.Project, when)
	})

	r.renderDays(w)

	if len(r.Unknown) > 0 {
		models := make([]string, 0, len(r.Unknown))
		for m := range r.Unknown {
			models = append(models, m)
		}
		sort.Strings(models)
		fmt.Fprintf(w, "\n  ⚠ no pricing for: %s\n", strings.Join(models, ", "))
		fmt.Fprintf(w, "    tokens counted, cost excluded — add rates in ~/.config/burned/pricing.json\n")
	}
	fmt.Fprintln(w)
}

func renderTable(w io.Writer, m map[string]*bucket, top int, label func(string) string) {
	type row struct {
		key string
		b   *bucket
	}
	rows := make([]row, 0, len(m))
	var totalCost float64
	for k, b := range m {
		rows = append(rows, row{k, b})
		totalCost += b.Cost
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].b.Cost != rows[j].b.Cost {
			return rows[i].b.Cost > rows[j].b.Cost
		}
		return rows[i].b.Usage.Total() > rows[j].b.Usage.Total()
	})
	if len(rows) > top {
		rows = rows[:top]
	}
	for _, rw := range rows {
		name := rw.key
		if label != nil {
			name = label(rw.key)
		}
		pct := 0.0
		if totalCost > 0 {
			pct = rw.b.Cost / totalCost * 100
		}
		fmt.Fprintf(w, "  %9s  %4.0f%%  %8s  %s\n",
			dollars(rw.b.Cost), pct, humanTokens(rw.b.Usage.Total()), name)
	}
}

func (r *Report) renderDays(w io.Writer) {
	const days = 14
	if len(r.ByDay) == 0 {
		return
	}
	fmt.Fprintf(w, "\n  LAST %d DAYS\n", days)
	var maxCost float64
	keys := make([]string, 0, days)
	for i := days - 1; i >= 0; i-- {
		key := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		keys = append(keys, key)
		if b := r.ByDay[key]; b != nil && b.Cost > maxCost {
			maxCost = b.Cost
		}
	}
	if maxCost == 0 {
		fmt.Fprintf(w, "  (no usage)\n")
		return
	}
	for _, key := range keys {
		var cost float64
		if b := r.ByDay[key]; b != nil {
			cost = b.Cost
		}
		bars := int(cost / maxCost * 40)
		day, _ := time.Parse("2006-01-02", key)
		fmt.Fprintf(w, "  %s %9s  %s\n", day.Format("Mon 02 Jan"), dollars(cost), strings.Repeat("█", bars))
	}
}

func dollars(v float64) string {
	if v >= 100 {
		return fmt.Sprintf("$%.0f", v)
	}
	return fmt.Sprintf("$%.2f", v)
}

func humanTokens(n int64) string {
	switch {
	case n >= 1e9:
		return fmt.Sprintf("%.1fB", float64(n)/1e9)
	case n >= 1e6:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1e3:
		return fmt.Sprintf("%.1fK", float64(n)/1e3)
	}
	return fmt.Sprintf("%d", n)
}
