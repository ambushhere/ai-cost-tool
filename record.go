package main

// Record is one normalized cost line: a day, a billing source, and money.
type Record struct {
	Date      string  `json:"date"` // YYYY-MM-DD (UTC bucket start)
	Provider  string  `json:"provider"`
	Source    string  `json:"source"` // workspace:<id> | api_key:<id> | project:<id>
	Model     string  `json:"model,omitempty"`
	USD       float64 `json:"usd"`
	Estimated bool    `json:"estimated"` // true when derived from token counts x pricing table
	Tags      Tags    `json:"tags"`
	Tagged    bool    `json:"tagged"` // false when source wasn't found in config
}
