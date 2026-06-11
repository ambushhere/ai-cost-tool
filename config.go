package main

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Tags is the attribution triple every cost record gets resolved to.
type Tags struct {
	Team    string `yaml:"team" json:"team"`
	Feature string `yaml:"feature" json:"feature"`
	Env     string `yaml:"env" json:"env"`
}

func (t Tags) withDefaults(d Tags) Tags {
	if t.Team == "" {
		t.Team = d.Team
	}
	if t.Feature == "" {
		t.Feature = d.Feature
	}
	if t.Env == "" {
		t.Env = d.Env
	}
	return t
}

// ModelPrice is USD per 1M tokens. Zero cache fields fall back to
// standard multipliers of Input (0.1x read, 1.25x 5m write, 2x 1h write).
type ModelPrice struct {
	Input        float64 `yaml:"input"`
	Output       float64 `yaml:"output"`
	CacheRead    float64 `yaml:"cache_read"`
	CacheWrite5m float64 `yaml:"cache_write_5m"`
	CacheWrite1h float64 `yaml:"cache_write_1h"`
}

type Config struct {
	DefaultTags Tags `yaml:"default_tags"`

	Anthropic struct {
		AdminKeyEnv string          `yaml:"admin_key_env"`
		Workspaces  map[string]Tags `yaml:"workspaces"`
		APIKeys     map[string]Tags `yaml:"api_keys"`
	} `yaml:"anthropic"`

	OpenAI struct {
		AdminKeyEnv string          `yaml:"admin_key_env"`
		Projects    map[string]Tags `yaml:"projects"`
	} `yaml:"openai"`

	Pricing map[string]ModelPrice `yaml:"pricing"`
}

func loadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.DefaultTags.Team == "" {
		cfg.DefaultTags.Team = "untagged"
	}
	if cfg.DefaultTags.Feature == "" {
		cfg.DefaultTags.Feature = "untagged"
	}
	if cfg.DefaultTags.Env == "" {
		cfg.DefaultTags.Env = "unknown"
	}
	if cfg.Anthropic.AdminKeyEnv == "" {
		cfg.Anthropic.AdminKeyEnv = "ANTHROPIC_ADMIN_KEY"
	}
	if cfg.OpenAI.AdminKeyEnv == "" {
		cfg.OpenAI.AdminKeyEnv = "OPENAI_ADMIN_KEY"
	}
	return &cfg, nil
}

// Built-in pricing (USD per 1M tokens); config `pricing:` entries override.
var defaultPricing = map[string]ModelPrice{
	"claude-fable-5":    {Input: 10, Output: 50},
	"claude-opus-4-8":   {Input: 5, Output: 25},
	"claude-opus-4-7":   {Input: 5, Output: 25},
	"claude-opus-4-6":   {Input: 5, Output: 25},
	"claude-opus-4-5":   {Input: 5, Output: 25},
	"claude-sonnet-4-6": {Input: 3, Output: 15},
	"claude-sonnet-4-5": {Input: 3, Output: 15},
	"claude-haiku-4-5":  {Input: 1, Output: 5},
}

// priceFor resolves a model price: config exact match, config prefix match,
// then the built-in table (exact, then prefix to absorb dated IDs like
// claude-haiku-4-5-20251001).
func (c *Config) priceFor(model string) (ModelPrice, bool) {
	lookup := func(table map[string]ModelPrice) (ModelPrice, bool) {
		if p, ok := table[model]; ok {
			return p, true
		}
		best := ""
		var bp ModelPrice
		for id, p := range table {
			if strings.HasPrefix(model, id) && len(id) > len(best) {
				best, bp = id, p
			}
		}
		return bp, best != ""
	}
	if p, ok := lookup(c.Pricing); ok {
		return p.fill(), true
	}
	if p, ok := lookup(defaultPricing); ok {
		return p.fill(), true
	}
	return ModelPrice{}, false
}

func (p ModelPrice) fill() ModelPrice {
	if p.CacheRead == 0 {
		p.CacheRead = p.Input * 0.1
	}
	if p.CacheWrite5m == 0 {
		p.CacheWrite5m = p.Input * 1.25
	}
	if p.CacheWrite1h == 0 {
		p.CacheWrite1h = p.Input * 2
	}
	return p
}

// tagsFor maps a billing source to tags. ok=false means the source was not
// found in the config and default tags were applied.
func (c *Config) tagsFor(provider, kind, id string) (Tags, bool) {
	var t Tags
	var ok bool
	switch provider + "/" + kind {
	case "anthropic/workspace":
		t, ok = c.Anthropic.Workspaces[id]
	case "anthropic/api_key":
		t, ok = c.Anthropic.APIKeys[id]
	case "openai/project":
		t, ok = c.OpenAI.Projects[id]
	}
	return t.withDefaults(c.DefaultTags), ok
}
