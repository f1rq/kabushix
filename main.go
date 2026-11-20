package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
type tickMsg time.Time

type model struct {
	tickers  []string
	quotes   map[string]quoteData
	selected int
	loading  bool
}

var (
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	normalStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	upStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	downStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	boxStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("238")).Padding(0, 1)
)

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
			return quoteMsg{Symbol: symbol, Err: fmt.Errorf("no data")}
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

func scheduleTick() tea.Cmd {
	return tea.Tick(30*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
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

func initialModel(tickers []string) model {
	quotes := make(map[string]quoteData, len(tickers))
	for _, t := range tickers {
		quotes[t] = quoteData{Symbol: t}
	}
	return model{
		tickers:  tickers,
		quotes:   quotes,
		selected: 0,
		loading:  true,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(fetchAll(m.tickers), scheduleTick())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		m.loading = true
		return m, tea.Batch(fetchAll(m.tickers), scheduleTick())

	case quoteMsg:
		m.quotes[msg.Symbol] = quoteData(msg)
		m.loading = false

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			idx := msg.Y - 1
			if idx >= 0 && idx < len(m.tickers) {
				m.selected = idx
				m.loading = true
				return m, fetchQuote(m.tickers[m.selected])
			}
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			if m.selected < len(m.tickers)-1 {
				m.selected++
			}
		case "k", "up":
			if m.selected > 0 {
				m.selected--
			}
		case "r":
			m.loading = true
			return m, fetchAll(m.tickers)
		}
	}
	return m, nil
}

func (m model) View() string {
	var rows strings.Builder
	for i, sym := range m.tickers {
		q := m.quotes[sym]
		cursor := "  "
		rowStyle := normalStyle
		if i == m.selected {
			cursor = "▸ "
			rowStyle = selectedStyle
		}

		var line string
		if q.Err != nil {
			line = fmt.Sprintf("%s%-10s %s", cursor, sym, downStyle.Render("error: "+q.Err.Error()))
		} else if q.Price == 0 && len(q.Series) == 0 {
			line = fmt.Sprintf("%s%-10s %s", cursor, sym, dimStyle.Render("loading..."))
		} else {
			sign := "+"
			pctStyle := upStyle
			if q.ChangePct < 0 {
				sign = ""
				pctStyle = downStyle
			}
			pctStr := pctStyle.Render(fmt.Sprintf("%s%.2f%%", sign, q.ChangePct))
			spark := dimStyle.Render(sparkline(q.Series, 16))
			line = fmt.Sprintf("%s%-10s %9.2f   %8s   %s", cursor, sym, q.Price, pctStr, spark)
		}

		rows.WriteString(rowStyle.Render(line) + "\n")
	}

	activeSym := m.tickers[m.selected]
	active := m.quotes[activeSym]
	detail := fmt.Sprintf(
		"Selected: %s |  Prev. close: %.2f | Punktów 1D: %d",
		selectedStyle.Render(activeSym),
		active.PrevClose,
		len(active.Series),
	)

	status := "click row or use ↑/↓ · r: refresh · q: quit"
	if m.loading {
		status = "downloading data..."
	}

	return boxStyle.Render(rows.String()) + "\n" +
		dimStyle.Render(detail) + "\n" +
		dimStyle.Render(status) + "\n"
}

func main() {
	tickers := []string{"NVDA", "AAPL", "MSFT", "CDR.WA", "BTC-USD"}
	p := tea.NewProgram(
		initialModel(tickers),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Println("Error:", err)
	}
}
