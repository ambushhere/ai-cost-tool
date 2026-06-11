package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// OpenAI Costs API: real billed USD per day, grouped by project.
// https://platform.openai.com/docs/api-reference/usage/costs
type openaiCostResp struct {
	Data []struct {
		StartTime int64 `json:"start_time"`
		Results   []struct {
			Amount struct {
				Value    float64 `json:"value"`
				Currency string  `json:"currency"`
			} `json:"amount"`
			ProjectID string `json:"project_id"`
			LineItem  string `json:"line_item"`
		} `json:"results"`
	} `json:"data"`
	HasMore  bool   `json:"has_more"`
	NextPage string `json:"next_page"`
}

func fetchOpenAICost(cfg *Config, adminKey string, from, to time.Time) ([]Record, error) {
	q := url.Values{}
	q.Set("start_time", fmt.Sprint(from.UTC().Unix()))
	q.Set("end_time", fmt.Sprint(to.UTC().Unix()))
	q.Set("bucket_width", "1d")
	q.Set("limit", "180")
	q.Add("group_by", "project_id")

	var records []Record
	for {
		req, err := http.NewRequest("GET", "https://api.openai.com/v1/organization/costs?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+adminKey)
		req.Header.Set("User-Agent", "aicost/0.1.0")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("openai costs: HTTP %d: %s", resp.StatusCode, truncate(string(body), 300))
		}
		var page openaiCostResp
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, err
		}
		for _, bucket := range page.Data {
			date := time.Unix(bucket.StartTime, 0).UTC().Format("2006-01-02")
			for _, r := range bucket.Results {
				proj := r.ProjectID
				if proj == "" {
					proj = "default"
				}
				tags, tagged := cfg.tagsFor("openai", "project", proj)
				records = append(records, Record{
					Date:     date,
					Provider: "openai",
					Source:   "project:" + proj,
					USD:      r.Amount.Value,
					Tags:     tags,
					Tagged:   tagged,
				})
			}
		}
		if !page.HasMore || page.NextPage == "" {
			break
		}
		q.Set("page", page.NextPage)
	}
	return records, nil
}
