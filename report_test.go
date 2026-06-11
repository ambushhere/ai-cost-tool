package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAggregateAndRender(t *testing.T) {
	recs := []Record{
		{Date: "2026-06-10", Provider: "anthropic", Source: "api_key:a", USD: 12.50,
			Tags: Tags{Team: "backend", Feature: "chat", Env: "prod"}, Tagged: true},
		{Date: "2026-06-11", Provider: "anthropic", Source: "api_key:a", USD: 7.50,
			Tags: Tags{Team: "backend", Feature: "chat", Env: "prod"}, Tagged: true},
		{Date: "2026-06-11", Provider: "openai", Source: "project:p", USD: 3.00,
			Tags: Tags{Team: "data", Feature: "rag", Env: "staging"}, Tagged: true},
	}
	rows := aggregate(recs, []string{"team", "feature", "env"})
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].USD != 20.0 || rows[0].Dims["team"] != "backend" {
		t.Fatalf("bad top row: %+v", rows[0])
	}
	var buf bytes.Buffer
	renderTable(&buf, []string{"team", "feature", "env"}, rows)
	out := buf.String()
	for _, want := range []string{"backend", "chat", "20.00", "TOTAL", "23.00"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q:\n%s", want, out)
		}
	}
}

func TestConfigAndPricing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(`
default_tags: {team: nobody}
anthropic:
  workspaces:
    wrkspc_x: {team: backend, env: prod}
  api_keys:
    apikey_y: {team: backend, feature: chat, env: prod}
pricing:
  my-custom-model: {input: 2, output: 8}
`), 0o644)

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	tags, ok := cfg.tagsFor("anthropic", "workspace", "wrkspc_x")
	if !ok || tags.Team != "backend" || tags.Feature != "untagged" {
		t.Fatalf("workspace tags wrong: %+v ok=%v", tags, ok)
	}
	if _, ok := cfg.tagsFor("anthropic", "api_key", "missing"); ok {
		t.Fatal("missing key should not be tagged")
	}

	// Built-in pricing, prefix match on a dated model ID.
	p, ok := cfg.priceFor("claude-haiku-4-5-20251001")
	if !ok || p.Input != 1 || p.Output != 5 {
		t.Fatalf("haiku pricing wrong: %+v ok=%v", p, ok)
	}
	if p.CacheRead != 0.1 || p.CacheWrite5m != 1.25 || p.CacheWrite1h != 2 {
		t.Fatalf("cache multipliers wrong: %+v", p)
	}
	// Config override wins.
	p, ok = cfg.priceFor("my-custom-model")
	if !ok || p.Input != 2 {
		t.Fatalf("custom pricing wrong: %+v ok=%v", p, ok)
	}
	if _, ok := cfg.priceFor("totally-unknown"); ok {
		t.Fatal("unknown model should have no price")
	}
}
