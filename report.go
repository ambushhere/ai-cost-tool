package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

type Row struct {
	Dims map[string]string `json:"dims"`
	USD  float64           `json:"usd"`
}

var validDims = map[string]bool{
	"team": true, "feature": true, "env": true,
	"provider": true, "model": true, "date": true, "source": true,
}

func dimValue(r Record, dim string) string {
	switch dim {
	case "team":
		return r.Tags.Team
	case "feature":
		return r.Tags.Feature
	case "env":
		return r.Tags.Env
	case "provider":
		return r.Provider
	case "model":
		if r.Model == "" {
			return "-"
		}
		return r.Model
	case "date":
		return r.Date
	case "source":
		return r.Source
	}
	return ""
}

func aggregate(records []Record, dims []string) []Row {
	byKey := map[string]*Row{}
	for _, rec := range records {
		vals := make([]string, len(dims))
		for i, d := range dims {
			vals[i] = dimValue(rec, d)
		}
		key := strings.Join(vals, "\x00")
		row, ok := byKey[key]
		if !ok {
			row = &Row{Dims: map[string]string{}}
			for i, d := range dims {
				row.Dims[d] = vals[i]
			}
			byKey[key] = row
		}
		row.USD += rec.USD
	}
	rows := make([]Row, 0, len(byKey))
	for _, r := range byKey {
		rows = append(rows, *r)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].USD > rows[j].USD })
	return rows
}

func renderTable(w io.Writer, dims []string, rows []Row) {
	widths := make([]int, len(dims))
	for i, d := range dims {
		widths[i] = len(d)
	}
	for _, r := range rows {
		for i, d := range dims {
			if n := len(r.Dims[d]); n > widths[i] {
				widths[i] = n
			}
		}
	}
	header := make([]string, 0, len(dims)+1)
	for i, d := range dims {
		header = append(header, pad(strings.ToUpper(d), widths[i]))
	}
	header = append(header, "        USD")
	fmt.Fprintln(w, strings.Join(header, "  "))

	var total float64
	for _, r := range rows {
		cells := make([]string, 0, len(dims)+1)
		for i, d := range dims {
			cells = append(cells, pad(r.Dims[d], widths[i]))
		}
		cells = append(cells, fmt.Sprintf("%11.2f", r.USD))
		fmt.Fprintln(w, strings.Join(cells, "  "))
		total += r.USD
	}
	sep := 0
	for _, n := range widths {
		sep += n + 2
	}
	fmt.Fprintln(w, strings.Repeat("-", sep+11))
	fmt.Fprintf(w, "%s  %11.2f\n", pad("TOTAL", sep-2), total)
}

func renderCSV(w io.Writer, dims []string, rows []Row) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(append(append([]string{}, dims...), "usd")); err != nil {
		return err
	}
	for _, r := range rows {
		rec := make([]string, 0, len(dims)+1)
		for _, d := range dims {
			rec = append(rec, r.Dims[d])
		}
		rec = append(rec, fmt.Sprintf("%.4f", r.USD))
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func renderJSON(w io.Writer, rows []Row) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rows)
}

func pad(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}
