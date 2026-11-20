package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type configData struct {
	Tickers []string `json:"tickers"`
}

func defaultTickers() []string {
	return []string{"NVDA", "AAPL", "MSFT", "CDR.WA", "BTC-USD"}
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tape", "watchlist.json"), nil
}

func loadTickers() []string {
	path, err := configPath()
	if err != nil {
		return defaultTickers()
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return defaultTickers()
	}

	var cfg configData
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return defaultTickers()
	}

	cleaned := normalizeTickers(cfg.Tickers)
	if len(cleaned) == 0 {
		return defaultTickers()
	}
	return cleaned
}

func saveTickers(tickers []string) error {
	path, err := configPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	cfg := configData{
		Tickers: normalizeTickers(tickers),
	}
	raw, err := json.MarshalIndent(cfg, "", " ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')

	return os.WriteFile(path, raw, 0o644)
}

func normalizeTickers(tickers []string) []string {
	seen := make(map[string]struct{}, len(tickers))
	out := make([]string, 0, len(tickers))

	for _, t := range tickers {
		sym := strings.ToUpper(strings.TrimSpace(t))
		if sym == "" {
			continue
		}
		if _, exists := seen[sym]; exists {
			continue
		}
		seen[sym] = struct{}{}
		out = append(out, sym)
	}
	return out
}
