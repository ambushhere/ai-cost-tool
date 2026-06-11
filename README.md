# aicost

CLI для управления затратами на AI: тянет расходы из биллинг-API провайдеров
(Anthropic, OpenAI), привязывает их к тегам **team / feature / env** через
конфиг-файл и строит отчёты.

## Как это работает

Провайдеры не знают про ваши фичи и команды — они считают деньги по своим
billing-источникам:

| Провайдер | Источник | Эндпоинт |
|---|---|---|
| Anthropic | workspace (реальные $) | `GET /v1/organizations/cost_report` |
| Anthropic | API key (токены × прайс, оценка) | `GET /v1/organizations/usage_report/messages` |
| OpenAI | project (реальные $) | `GET /v1/organization/costs` |

`aicost` маппит эти источники на теги из `config.yaml`. Поэтому базовая
дисциплина: **один API-ключ (или workspace/project) на комбинацию
фича+окружение** — тогда атрибуция точная.

## Установка

```sh
cd aicost
go build -o aicost.exe .
```

## Настройка

1. Получите admin-ключи:
   - Anthropic: Console → Settings → Admin keys (`sk-ant-admin...`). Нужна
     организация (на individual-аккаунтах Admin API недоступен).
   - OpenAI: platform.openai.com → Organization → Admin keys.
2. Экспортируйте их:
   ```powershell
   $env:ANTHROPIC_ADMIN_KEY = "sk-ant-admin..."
   $env:OPENAI_ADMIN_KEY    = "sk-admin..."
   ```
3. `cp config.example.yaml config.yaml` и опишите маппинг
   workspace/ключ/project → теги.

## Использование

```sh
# Реальные затраты за последние 7 дней по team/feature/env
./aicost

# За период, в разрезе команда+окружение
./aicost -from 2026-06-01 -to 2026-06-12 -group team,env

# Per-feature атрибуция через API-ключи Anthropic (оценка по токенам)
./aicost -mode usage -group feature,env

# Разрез по моделям и провайдерам
./aicost -group provider,model -format csv > costs.csv

# Какие источники ещё не затегированы (и сколько они стоят)
./aicost -untagged
```

Флаги: `-config`, `-from`, `-to`, `-group` (team,feature,env,provider,model,date,source),
`-mode cost|usage`, `-format table|csv|json`, `-providers anthropic,openai`, `-untagged`.

## Режимы

- **`-mode cost`** (по умолчанию) — реальные счёта в USD. Гранулярность:
  workspace (Anthropic) / project (OpenAI). Подходит для финансов и chargeback.
- **`-mode usage`** — токены по каждому API-ключу Anthropic × встроенная
  таблица цен. Это **оценка** (не учитывает скидки, кредиты, server tools),
  зато даёт атрибуцию на уровне фичи, если ключи разведены по фичам.
  Ключ без тега наследует теги своего workspace.

Данные в API появляются с задержкой ~5 минут; опрос чаще раза в минуту не
рекомендуется (для cron-отчётов это неважно).

## Идеи для развития

- cron-режим + порог бюджета с алертом (exit code / webhook в Slack)
- экспорт в Prometheus pushgateway / Grafana
- Claude Code Analytics API для per-user разбивки затрат разработчиков
