# burned

**See where your AI coding-agent tokens went.**

Your agent sessions burn tokens invisibly — until the bill (or the rate limit) hits. `burned` X-rays the session logs already sitting on your machine and shows you, in API-equivalent dollars: which models, which projects, and which individual sessions ate your budget.

```
  burned — where your tokens went
  3 Feb 2026 → 11 Jul 2026

  API-equivalent value   $7077
  Total tokens           8.9B across 323 sessions (47467 requests)
  Saved by caching       $31293 (would be $38370 uncached)

  BY MODEL
      $4126    58%      4.4B  claude-opus-4-8
       $903    13%    483.7M  claude-fable-5
       $865    12%      1.9B  gpt-5.4
       ...

  MOST EXPENSIVE SESSIONS
       $776    11%    727.8M  d40c476c  ~/Projects/acme-web (13 Jun)
       $495     7%    595.7M  ea3301b5  ~/Projects/side-app (2 Jun)
       ...
```

On a subscription plan? The number is your plan's *API-equivalent value* — what the same work would have cost pay-as-you-go. (Your $200/mo Max plan doing $2,000 of API work is a stat worth knowing.)

## Supported agents

- **Claude Code** — `~/.claude/projects/**/*.jsonl` (per-message usage, full cache breakdown, deduplicated across streaming updates)
- **Codex CLI** — `~/.codex/sessions/**/*.jsonl` (cumulative token counters, delta-accounted)
- Cursor is not supported: its token accounting happens server-side and isn't in local logs.

## Install

```sh
go install github.com/phrypy/burned@latest
```

## Usage

```sh
burned                    # everything, all time
burned -since 7d          # last week
burned -project acme      # one project
burned -source claude     # one agent
burned -json              # machine-readable aggregate
```

## Privacy

`burned` is strictly read-only and makes **zero network calls**. Your logs contain your code and prompts; nothing leaves your machine. The binary has no telemetry, no update check, nothing.

## Pricing accuracy

Costs use published API rates (as of 2026-07): Claude cache reads at 0.1× input, cache writes at 1.25×/2× for 5m/1h TTL; OpenAI cached input at 0.1× input. Models without a known rate are reported with tokens counted and cost excluded, never guessed. Override or extend rates in `~/.config/burned/pricing.json`:

```json
{ "gpt-5.6-sol": { "input": 5, "output": 30 } }
```

## Notes on accuracy

- Claude Code streams append the same message repeatedly with cumulative usage; `burned` counts each `(message.id, requestId)` once.
- Codex `token_count` events repeat; `burned` sums deltas of the cumulative counter, not per-event values.
- Subagent (sidechain) usage is real usage and is included.
- Worktree checkouts (`.claude/worktrees/...`) fold into their parent project.
