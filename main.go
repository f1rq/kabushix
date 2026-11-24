package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const refreshInterval = 30 * time.Second

type tickMsg time.Time

type model struct {
	tickers     []string
	quotes      map[string]quoteData
	selected    int
	loading     bool
	now         time.Time
	nextRefresh time.Time
	adding      bool
	input       string
	notice      string
}

var (
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	normalStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	upStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	downStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	accentStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
	boxStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("238")).Padding(0, 1)
)

func scheduleTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
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
	now := time.Now()
	m.now = now
	m.nextRefresh = now.Add(refreshInterval)
	return tea.Batch(fetchAll(m.tickers), scheduleTick())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		m.now = time.Time(msg)
		if m.adding {
			return m, scheduleTick()
		}
		if !m.now.Before(m.nextRefresh) {
			m.nextRefresh = m.now.Add(refreshInterval)
			m.loading = true
			return m, tea.Batch(fetchAll(m.tickers), scheduleTick())
		}
		return m, scheduleTick()

	case quoteMsg:
		m.quotes[msg.Symbol] = quoteData(msg)
		m.loading = false

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			idx := msg.Y - 1
			if idx >= 0 && idx < len(m.tickers) {
				m.selected = idx
				m.notice = ""
				m.loading = true
				return m, fetchQuote(m.tickers[m.selected])
			}
		}

	case tea.KeyMsg:
		if m.adding {
			return m.handleAddKey(msg)
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			if m.selected < len(m.tickers)-1 {
				m.selected++
				m.notice = ""
			}
		case "k", "up":
			if m.selected > 0 {
				m.selected--
				m.notice = ""
			}
		case "r":
			now := time.Now()
			m.now = now
			m.nextRefresh = now.Add(refreshInterval)
			m.loading = true
			m.notice = ""
			return m, fetchAll(m.tickers)
		case "+", "a":
			m.adding = true
			m.input = ""
			m.notice = ""
		case "-":
			return m, m.removeSelected()
		}
	}
	return m, nil
}

func (m model) handleAddKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.adding = false
		m.input = ""
		return m, nil

	case tea.KeyEnter:
		sym := strings.ToUpper(strings.TrimSpace(m.input))
		m.adding = false
		m.input = ""
		if sym == "" {
			return m, nil
		}
		for i, existing := range m.tickers {
			if existing == sym {
				m.selected = i
				m.notice = sym + " is already added"
				return m, nil
			}
		}

		m.tickers = append(m.tickers, sym)
		m.quotes[sym] = quoteData{Symbol: sym}
		m.selected = len(m.tickers) - 1
		m.loading = true

		if err := saveTickers(m.tickers); err != nil {
			m.notice = "write error: " + err.Error()
		} else {
			m.notice = "added " + sym
		}
		return m, fetchQuote(sym)
	case tea.KeyBackspace, tea.KeyDelete:
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
		return m, nil

	case tea.KeyRunes:
		if msg.String() == " " {
			return m, nil
		}
		m.input += strings.ToUpper(msg.String())
		return m, nil
	}
	return m, nil
}

func (m *model) removeSelected() tea.Cmd {
	if len(m.tickers) <= 1 {
		m.notice = "you need to have at least one ticker"
		return nil
	}

	removed := m.tickers[m.selected]
	m.tickers = append(m.tickers[:m.selected], m.tickers[m.selected+1:]...)
	delete(m.quotes, removed)

	if m.selected >= len(m.tickers) {
		m.selected = len(m.tickers) - 1
	}

	if err := saveTickers(m.tickers); err != nil {
		m.notice = "write error: " + err.Error()
	} else {
		m.notice = "removed " + removed
	}
	return nil
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
		"Selected: %s | Prev. close: %.2f | 1D Points: %d",
		selectedStyle.Render(activeSym),
		active.PrevClose,
		len(active.Series),
	)

	var extraLine string
	if m.adding {
		extraLine = accentStyle.Render(fmt.Sprintf("New ticker: %s█", m.input)) + dimStyle.Render("  (Enter: add, Esc: cancel)")
	} else if m.notice != "" {
		extraLine = accentStyle.Render(m.notice)
	}

	status := "+: add · -: remove · r: refresh · click/↑↓ · q: quit"
	if m.loading {
		status = fmt.Sprintf("downloading data...")
	}

	now := m.now
	if now.IsZero() {
		now = time.Now()
	}

	plLoc, errPL := time.LoadLocation("Europe/Warsaw")
	usLoc, errUS := time.LoadLocation("America/New_York")

	plTime := now.Format("15:04:05")
	if errPL == nil {
		plTime = now.In(plLoc).Format("15:04:05")
	}

	usTime := now.Format("15:04:05")
	if errUS == nil {
		usTime = now.In(usLoc).Format("15:04:05")
	}

	secsLeft := int(time.Until(m.nextRefresh).Seconds())
	if secsLeft < 0 {
		secsLeft = 0
	}

	timeLine := fmt.Sprintf("PL: %s  |  US (ET): %s  |  next refresh in: %ds", plTime, usTime, secsLeft)

	out := boxStyle.Render(rows.String()) + "\n" + dimStyle.Render(detail) + "\n"
	if extraLine != "" {
		out += extraLine + "\n"
	}
	out += dimStyle.Render(status) + "\n"
	out += dimStyle.Render(timeLine) + "\n"
	return out
}

func main() {
	tickers := loadTickers()
	p := tea.NewProgram(
		initialModel(tickers),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Println("Error:", err)
	}
}
