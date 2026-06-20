# aicost

A CLI for AI cost management: pulls spend from provider billing APIs
(Anthropic, OpenAI), attributes it to **team / feature / env** tags via a
config file, and produces reports.

## Quick demo

No API keys needed — run the full pipeline against sample data:

```sh
cd aicost
go build -o aicost.exe .

# tags are resolved from this file
cp config.example.yaml config.yaml

# run the CLI on bundled test data
./aicost -fixture fixture.example.json -group team,feature,env
```

Output:

```text
TEAM      FEATURE         ENV              USD
backend   chat-assistant  prod           80.65
data      rag-pipeline    staging        11.20
backend   summarizer      prod            7.80
untagged  untagged        unknown         6.45
----------------------------------------------
TOTAL                                   106.10
```

The same pipeline runs against live billing data once you provide an admin key
(see [Setup](#setup)) — only the data source changes, the tagging and reports
are identical. More group/format variations:

```sh
./aicost -fixture fixture.example.json -group provider,model
./aicost -fixture fixture.example.json -group date,source -format csv
./aicost -fixture fixture.example.json -untagged
```

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

The [Quick demo](#quick-demo) above already runs the CLI on bundled data via
`-fixture`. Fixture mode loads records from a JSON file and re-resolves their
tags through your `config.yaml`, so every `-group` / `-format` / `-untagged`
path is exercised without touching the network.

```sh
# Unit tests (aggregation, tagging, pricing, rendering)
go test ./...
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
