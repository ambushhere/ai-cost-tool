package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const anthropicBase = "https://api.anthropic.com"

func anthropicGet(adminKey, path string, q url.Values, out any) error {
	req, err := http.NewRequest("GET", anthropicBase+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", adminKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("User-Agent", "aicost/0.1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("anthropic %s: HTTP %d: %s", path, resp.StatusCode, truncate(string(body), 300))
	}
	return json.Unmarshal(body, out)
}

// ---- Cost report (real billed USD, workspace granularity) ----

type anthropicCostResp struct {
	Data []struct {
		StartingAt string `json:"starting_at"`
		Results    []struct {
			Amount      string `json:"amount"` // cents, decimal string
			Currency    string `json:"currency"`
			WorkspaceID string `json:"workspace_id"`
			Model       string `json:"model"`
			CostType    string `json:"cost_type"`
		} `json:"results"`
	} `json:"data"`
	HasMore  bool   `json:"has_more"`
	NextPage string `json:"next_page"`
}

func fetchAnthropicCost(cfg *Config, adminKey string, from, to time.Time) ([]Record, error) {
	q := url.Values{}
	q.Set("starting_at", from.UTC().Format(time.RFC3339))
	q.Set("ending_at", to.UTC().Format(time.RFC3339))
	q.Set("bucket_width", "1d")
	q.Set("limit", "31")
	q.Add("group_by[]", "workspace_id")
	q.Add("group_by[]", "description")

	var records []Record
	for {
		var resp anthropicCostResp
		if err := anthropicGet(adminKey, "/v1/organizations/cost_report", q, &resp); err != nil {
			return nil, err
		}
		for _, bucket := range resp.Data {
			date := bucket.StartingAt[:10]
			for _, r := range bucket.Results {
				cents, err := strconv.ParseFloat(r.Amount, 64)
				if err != nil {
					continue
				}
				ws := r.WorkspaceID
				if ws == "" {
					ws = "default"
				}
				tags, tagged := cfg.tagsFor("anthropic", "workspace", ws)
				records = append(records, Record{
					Date:     date,
					Provider: "anthropic",
					Source:   "workspace:" + ws,
					Model:    r.Model,
					USD:      cents / 100,
					Tags:     tags,
					Tagged:   tagged,
				})
			}
		}
		if !resp.HasMore || resp.NextPage == "" {
			break
		}
		q.Set("page", resp.NextPage)
	}
	return records, nil
}

// ---- Usage report (token counts per API key, estimated USD) ----

type anthropicUsageResp struct {
	Data []struct {
		StartingAt string `json:"starting_at"`
		Results    []struct {
			APIKeyID      string `json:"api_key_id"`
			WorkspaceID   string `json:"workspace_id"`
			Model         string `json:"model"`
			Uncached      int64  `json:"uncached_input_tokens"`
			CacheRead     int64  `json:"cache_read_input_tokens"`
			OutputTokens  int64  `json:"output_tokens"`
			CacheCreation struct {
				Ephemeral5m int64 `json:"ephemeral_5m_input_tokens"`
				Ephemeral1h int64 `json:"ephemeral_1h_input_tokens"`
			} `json:"cache_creation"`
		} `json:"results"`
	} `json:"data"`
	HasMore  bool   `json:"has_more"`
	NextPage string `json:"next_page"`
}

func fetchAnthropicUsage(cfg *Config, adminKey string, from, to time.Time) ([]Record, error) {
	q := url.Values{}
	q.Set("starting_at", from.UTC().Format(time.RFC3339))
	q.Set("ending_at", to.UTC().Format(time.RFC3339))
	q.Set("bucket_width", "1d")
	q.Set("limit", "31")
	q.Add("group_by[]", "api_key_id")
	q.Add("group_by[]", "workspace_id")
	q.Add("group_by[]", "model")

	var records []Record
	for {
		var resp anthropicUsageResp
		if err := anthropicGet(adminKey, "/v1/organizations/usage_report/messages", q, &resp); err != nil {
			return nil, err
		}
		for _, bucket := range resp.Data {
			date := bucket.StartingAt[:10]
			for _, r := range bucket.Results {
				price, ok := cfg.priceFor(r.Model)
				if !ok {
					fmt.Fprintf(warnOut, "warning: no pricing for model %q — usage skipped; add it under pricing: in the config\n", r.Model)
					continue
				}
				usd := (float64(r.Uncached)*price.Input +
					float64(r.CacheRead)*price.CacheRead +
					float64(r.CacheCreation.Ephemeral5m)*price.CacheWrite5m +
					float64(r.CacheCreation.Ephemeral1h)*price.CacheWrite1h +
					float64(r.OutputTokens)*price.Output) / 1e6

				source, kind, id := "api_key:"+r.APIKeyID, "api_key", r.APIKeyID
				if r.APIKeyID == "" { // Workbench/Console usage has no key
					ws := r.WorkspaceID
					if ws == "" {
						ws = "default"
					}
					source, kind, id = "workspace:"+ws, "workspace", ws
				}
				tags, tagged := cfg.tagsFor("anthropic", kind, id)
				// API key not tagged directly — fall back to its workspace tags.
				if !tagged && kind == "api_key" && r.WorkspaceID != "" {
					tags, tagged = cfg.tagsFor("anthropic", "workspace", r.WorkspaceID)
				}
				records = append(records, Record{
					Date:      date,
					Provider:  "anthropic",
					Source:    source,
					Model:     r.Model,
					USD:       usd,
					Estimated: true,
					Tags:      tags,
					Tagged:    tagged,
				})
			}
		}
		if !resp.HasMore || resp.NextPage == "" {
			break
		}
		q.Set("page", resp.NextPage)
	}
	return records, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
