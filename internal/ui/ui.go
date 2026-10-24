// Package ui renders the google-chrome-tab-manager terminal interface.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/maxjacobson/google-chrome-tab-manager/internal/chrome"
)

const (
	// loadTimeout bounds one attempt to read Chrome's tabs.
	loadTimeout = 30 * time.Second
	// spinnerFPS is how often the loading spinner animates.
	spinnerFPS = 100 * time.Millisecond
)

var spinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("238"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	badStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
)

// Source lists Chrome's open windows. chrome.Chrome implements it; tests
// substitute fakes.
type Source interface {
	ListTabs(ctx context.Context) ([]chrome.Window, error)
}

// Model is the bubbletea model for the tab manager. It loads tabs once on
// startup, shows a spinner while it waits, and then lists the windows.
type Model struct {
	source Source
	ctx    context.Context

	loading      bool
	spinnerFrame int

	windows  []chrome.Window
	err      error
	selected int

	width, height int
}

// New builds a Model that loads tabs from source on startup.
func New(ctx context.Context, source Source) Model {
	return Model{source: source, ctx: ctx, loading: true}
}

// tabsMsg carries the open windows (or a listing error).
type tabsMsg struct {
	windows []chrome.Window
	err     error
}

// spinnerTickMsg advances the loading spinner.
type spinnerTickMsg struct{}

func (m Model) load() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, loadTimeout)
		defer cancel()
		windows, err := m.source.ListTabs(ctx)
		return tabsMsg{windows: windows, err: err}
	}
}

func (m Model) spinnerTick() tea.Cmd {
	return tea.Tick(spinnerFPS, func(time.Time) tea.Msg { return spinnerTickMsg{} })
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.load(), m.spinnerTick())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case spinnerTickMsg:
		if !m.loading {
			return m, nil
		}
		m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
		return m, m.spinnerTick()
	case tabsMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.windows = msg.windows
		m.selected = 0
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		}
		if m.loading || m.err != nil {
			return m, nil
		}
		switch msg.String() {
		case "j", "down":
			m.selected = min(m.selected+1, len(m.windows)-1)
		case "k", "up":
			m.selected = max(m.selected-1, 0)
		case "g":
			m.selected = 0
		case "G":
			m.selected = len(m.windows) - 1
		}
		return m, nil
	}
	return m, nil
}

func (m Model) View() tea.View {
	var content string
	switch {
	case m.err != nil:
		content = m.viewError()
	case m.loading:
		content = m.viewLoading()
	default:
		content = m.viewList()
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m Model) viewLoading() string {
	return fmt.Sprintf("%s %s\n", spinnerFrames[m.spinnerFrame], dimStyle.Render("asking Chrome for its tabs…"))
}

func (m Model) viewError() string {
	return badStyle.Render(fmt.Sprintf("error: %v", m.err)) + "\n\n" +
		dimStyle.Render("press q to quit")
}

func (m Model) viewList() string {
	totalTabs := 0
	for _, w := range m.windows {
		totalTabs += len(w.Tabs)
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf(
		"Google Chrome: %d %s, %d %s",
		len(m.windows), plural("window", len(m.windows)),
		totalTabs, plural("tab", totalTabs),
	)))
	b.WriteString("\n\n")
	for i, w := range m.windows {
		line := fmt.Sprintf("Window %d  %s", w.Index, dimStyle.Render(fmt.Sprintf("%d %s", len(w.Tabs), plural("tab", len(w.Tabs)))))
		if i == m.selected {
			b.WriteString(selectedStyle.Render("▸ " + line))
		} else {
			b.WriteString("  " + line)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("j/k select · q quit"))
	return b.String()
}

func plural(word string, n int) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
