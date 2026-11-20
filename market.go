package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type quoteData struct {
	Symbol    string
	Price     float64
	PrevClose float64
	ChangePct float64
	Series    []float64
	Err       error
}

type quoteMsg quoteData

var httpClient = &http.Client{Timeout: 8 * time.Second}

func fetchQuote(symbol string) tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=15m&range=1d", symbol)
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return quoteMsg{Symbol: symbol, Err: err}
		}
		req.Header.Set("User-Agent", "Mozilla/5.0")

		resp, err := httpClient.Do(req)
		if err != nil {
			return quoteMsg{Symbol: symbol, Err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return quoteMsg{Symbol: symbol, Err: fmt.Errorf("HTTP %d", resp.StatusCode)}
		}

		var payload struct {
			Chart struct {
				Result []struct {
					Meta struct {
						RegularMarketPrice float64 `json:"regularMarketPrice"`
						ChartPreviousClose float64 `json:"chartPreviousClose"`
					} `json:"meta"`
					Indicators struct {
						Quote []struct {
							Close []*float64 `json:"close"`
						} `json:"quote"`
					} `json:"indicators"`
				} `json:"result"`
			} `json:"chart"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return quoteMsg{Symbol: symbol, Err: err}
		}
		if len(payload.Chart.Result) == 0 {
			return quoteMsg{Symbol: symbol, Err: fmt.Errorf("brak danych")}
		}

		res := payload.Chart.Result[0]
		price := res.Meta.RegularMarketPrice
		prev := res.Meta.ChartPreviousClose

		var changePct float64
		if prev != 0 {
			changePct = ((price - prev) / prev) * 100
		}

		var series []float64
		if len(res.Indicators.Quote) > 0 {
			for _, v := range res.Indicators.Quote[0].Close {
				if v != nil {
					series = append(series, *v)
				}
			}
		}

		return quoteMsg{
			Symbol:    symbol,
			Price:     price,
			PrevClose: prev,
			ChangePct: changePct,
			Series:    series,
		}
	}
}

func fetchAll(tickers []string) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(tickers))
	for _, t := range tickers {
		cmds = append(cmds, fetchQuote(t))
	}
	return tea.Batch(cmds...)
}

func sparkline(values []float64, width int) string {
	if len(values) < 2 {
		return ""
	}
	if len(values) > width {
		values = values[len(values)-width:]
	}

	minVal, maxVal := values[0], values[0]
	for _, v := range values[1:] {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	bars := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	span := maxVal - minVal
	var sb strings.Builder

	for _, v := range values {
		if span == 0 {
			sb.WriteRune(bars[len(bars)/2])
			continue
		}
		idx := int(((v - minVal) / span) * float64(len(bars)-1))
		if idx < 0 {
			idx = 0
		} else if idx >= len(bars) {
			idx = len(bars) - 1
		}
		sb.WriteRune(bars[idx])
	}
	return sb.String()
}
