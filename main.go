package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

var warnOut = os.Stderr

func main() {
	var (
		cfgPath      = flag.String("config", "config.yaml", "path to config file")
		fromStr      = flag.String("from", "", "start date YYYY-MM-DD inclusive (default: 7 days ago)")
		toStr        = flag.String("to", "", "end date YYYY-MM-DD inclusive (default: today)")
		groupStr     = flag.String("group", "team,feature,env", "comma-separated dimensions: team,feature,env,provider,model,date,source")
		mode         = flag.String("mode", "cost", "cost = billed USD (workspace/project granularity) | usage = Anthropic per-API-key tokens priced via pricing table (estimated)")
		format       = flag.String("format", "table", "table | csv | json")
		providersStr = flag.String("providers", "", "comma-separated: anthropic,openai (default: every provider whose admin key env var is set)")
		showUntagged = flag.Bool("untagged", false, "list billing sources that have no tags in the config, then exit")
	)
	flag.Parse()

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		fatal("config: %v", err)
	}

	from, to, err := parseRange(*fromStr, *toStr)
	if err != nil {
		fatal("%v", err)
	}

	providers := resolveProviders(*providersStr, cfg)
	if len(providers) == 0 {
		fatal("no providers: set %s and/or %s, or pass -providers", cfg.Anthropic.AdminKeyEnv, cfg.OpenAI.AdminKeyEnv)
	}

	var records []Record
	for _, p := range providers {
		var recs []Record
		var err error
		switch p {
		case "anthropic":
			key := os.Getenv(cfg.Anthropic.AdminKeyEnv)
			if key == "" {
				fatal("env var %s is empty (Anthropic Admin API key, sk-ant-admin...)", cfg.Anthropic.AdminKeyEnv)
			}
			if *mode == "usage" {
				recs, err = fetchAnthropicUsage(cfg, key, from, to)
			} else {
				recs, err = fetchAnthropicCost(cfg, key, from, to)
			}
		case "openai":
			key := os.Getenv(cfg.OpenAI.AdminKeyEnv)
			if key == "" {
				fatal("env var %s is empty (OpenAI admin key)", cfg.OpenAI.AdminKeyEnv)
			}
			if *mode == "usage" {
				fmt.Fprintln(warnOut, "note: -mode usage is Anthropic-only; OpenAI always reports billed cost")
			}
			recs, err = fetchOpenAICost(cfg, key, from, to)
		default:
			fatal("unknown provider %q", p)
		}
		if err != nil {
			fatal("%s: %v", p, err)
		}
		records = append(records, recs...)
	}

	if *showUntagged {
		printUntagged(records)
		return
	}

	dims := splitList(*groupStr)
	for _, d := range dims {
		if !validDims[d] {
			fatal("unknown dimension %q (valid: team, feature, env, provider, model, date, source)", d)
		}
	}
	rows := aggregate(records, dims)

	switch *format {
	case "table":
		renderTable(os.Stdout, dims, rows)
		if *mode == "usage" {
			fmt.Fprintln(os.Stdout, "\n(USD values are estimates: token counts x pricing table, excludes discounts/credits/server tools)")
		}
	case "csv":
		if err := renderCSV(os.Stdout, dims, rows); err != nil {
			fatal("csv: %v", err)
		}
	case "json":
		if err := renderJSON(os.Stdout, rows); err != nil {
			fatal("json: %v", err)
		}
	default:
		fatal("unknown format %q", *format)
	}

	warnUntagged(records)
}

func parseRange(fromStr, toStr string) (time.Time, time.Time, error) {
	now := time.Now().UTC()
	from := now.AddDate(0, 0, -7).Truncate(24 * time.Hour)
	to := now
	var err error
	if fromStr != "" {
		from, err = time.Parse("2006-01-02", fromStr)
		if err != nil {
			return from, to, fmt.Errorf("bad -from %q: expected YYYY-MM-DD", fromStr)
		}
	}
	if toStr != "" {
		to, err = time.Parse("2006-01-02", toStr)
		if err != nil {
			return from, to, fmt.Errorf("bad -to %q: expected YYYY-MM-DD", toStr)
		}
		to = to.AddDate(0, 0, 1) // inclusive end date -> exclusive timestamp
	}
	if !to.After(from) {
		return from, to, fmt.Errorf("-to must not be before -from")
	}
	return from, to, nil
}

func resolveProviders(explicit string, cfg *Config) []string {
	if explicit != "" {
		return splitList(explicit)
	}
	var out []string
	if os.Getenv(cfg.Anthropic.AdminKeyEnv) != "" {
		out = append(out, "anthropic")
	}
	if os.Getenv(cfg.OpenAI.AdminKeyEnv) != "" {
		out = append(out, "openai")
	}
	return out
}

func printUntagged(records []Record) {
	seen := map[string]float64{}
	for _, r := range records {
		if !r.Tagged {
			seen[r.Provider+" "+r.Source] += r.USD
		}
	}
	if len(seen) == 0 {
		fmt.Println("All billing sources are tagged.")
		return
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return seen[keys[i]] > seen[keys[j]] })
	fmt.Println("Untagged billing sources (add them to the config):")
	for _, k := range keys {
		fmt.Printf("  %-70s %10.2f USD\n", k, seen[k])
	}
}

func warnUntagged(records []Record) {
	var untaggedUSD, totalUSD float64
	n := map[string]bool{}
	for _, r := range records {
		totalUSD += r.USD
		if !r.Tagged {
			untaggedUSD += r.USD
			n[r.Source] = true
		}
	}
	if len(n) > 0 && totalUSD > 0 {
		fmt.Fprintf(warnOut, "\nwarning: %d source(s) untagged — %.2f USD (%.0f%% of total). Run with -untagged to list them.\n",
			len(n), untaggedUSD, untaggedUSD/totalUSD*100)
	}
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "aicost: "+format+"\n", args...)
	os.Exit(1)
}
