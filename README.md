# aicost

A CLI for AI cost management: pulls spend from provider billing APIs
(Anthropic, OpenAI), attributes it to **team / feature / env** tags via a
config file, and produces reports.

## How it works

Providers don't know about your features and teams — they bill by their own
billing sources:

| Provider | Source | Endpoint |
|---|---|---|
| Anthropic | workspace (real $) | `GET /v1/organizations/cost_report` |
| Anthropic | API key (tokens × pricing, estimate) | `GET /v1/organizations/usage_report/messages` |
| OpenAI | project (real $) | `GET /v1/organization/costs` |

`aicost` maps these sources to tags from `config.yaml`. The core discipline
that makes this work: **one API key (or workspace/project) per
feature+environment combination** — then attribution is exact.

## Installation

```sh
cd aicost
go build -o aicost.exe .
```

## Setup

1. Get admin keys:
   - Anthropic: Console → Settings → Admin keys (`sk-ant-admin...`). Requires
     an organization (the Admin API is unavailable on individual accounts).
   - OpenAI: platform.openai.com → Organization → Admin keys.
2. Export them:
   ```powershell
   $env:ANTHROPIC_ADMIN_KEY = "sk-ant-admin..."
   $env:OPENAI_ADMIN_KEY    = "sk-admin..."
   ```
3. `cp config.example.yaml config.yaml` and describe the
   workspace/key/project → tags mapping.

## Usage

```sh
# Real costs for the last 7 days by team/feature/env
./aicost

# A date range, broken down by team+environment
./aicost -from 2026-06-01 -to 2026-06-12 -group team,env

# Per-feature attribution via Anthropic API keys (token-based estimate)
./aicost -mode usage -group feature,env

# Breakdown by model and provider
./aicost -group provider,model -format csv > costs.csv

# Which sources are still untagged (and how much they cost)
./aicost -untagged
```

Flags: `-config`, `-from`, `-to`, `-group` (team,feature,env,provider,model,date,source),
`-mode cost|usage`, `-format table|csv|json`, `-providers anthropic,openai`, `-untagged`,
`-fixture`.

## Local testing without admin keys

You don't need provider admin keys to verify the tool works. Two options:

```sh
# 1. Unit tests (aggregation, tagging, pricing, rendering)
go test ./...

# 2. Run the full CLI against sample records loaded from a JSON file.
#    Tags are re-resolved through your config.yaml, so the mapping is
#    exercised too — every -group / -format / -untagged path works.
cp config.example.yaml config.yaml
./aicost -fixture fixture.example.json -group team,feature,env
./aicost -fixture fixture.example.json -untagged
```

Each fixture record needs `provider`, `source` (`"<kind>:<id>"`, where kind is
`workspace` / `api_key` / `project`), and `usd`; `model` and `date` are
optional. See `fixture.example.json` for the shape.

> Note: a plain DeepSeek/OpenAI inference key won't work as a data source —
> this tool reads provider *billing* APIs (cost/usage reports grouped by
> key/workspace/project), which inference keys don't expose. Use fixture mode
> to test the pipeline locally.

## Modes

- **`-mode cost`** (default) — real billed USD. Granularity: workspace
  (Anthropic) / project (OpenAI). Suitable for finance and chargeback.
- **`-mode usage`** — token counts per Anthropic API key × a built-in pricing
  table. This is an **estimate** (doesn't account for discounts, credits, or
  server tools), but it gives feature-level attribution when keys are split
  per feature. An untagged key inherits the tags of its workspace.

Data appears in the APIs with a ~5 minute delay; polling more often than once
per minute is not recommended (irrelevant for cron reports).

## Ideas for future development

- cron mode + budget threshold with alerting (exit code / Slack webhook)
- export to Prometheus pushgateway / Grafana
- Claude Code Analytics API for per-user developer cost breakdowns
