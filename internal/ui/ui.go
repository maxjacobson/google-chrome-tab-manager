// Package ui renders the google-chrome-tab-manager terminal interface.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/maxjacobson/google-chrome-tab-manager/internal/chrome"
)

const (
	// loadTimeout bounds one attempt to read Chrome's tabs.
	loadTimeout = 30 * time.Second
	// activateTimeout bounds one attempt to focus a tab in Chrome.
	activateTimeout = 10 * time.Second
	// spinnerFPS is how often the loading spinner animates.
	spinnerFPS = 100 * time.Millisecond
	// mouseWheelLines is how many rows one wheel notch moves.
	mouseWheelLines = 3
)

var spinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("238"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	badStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
)

// Source lists Chrome's open windows and focuses tabs in Chrome.
// chrome.Chrome implements it; tests substitute fakes.
type Source interface {
	ListTabs(ctx context.Context) ([]chrome.Window, error)
	ActivateTab(ctx context.Context, windowIndex, tabIndex int) error
}

type mode int

const (
	modeWindows mode = iota
	modeTabs
)

// Model is the bubbletea model for the tab manager. It loads tabs once on
// startup, lists the windows, and lets the user drill into one window's tabs.
type Model struct {
	source Source
	ctx    context.Context

	loading      bool
	spinnerFrame int

	windows []chrome.Window
	err     error

	mode      mode
	winCursor int
	tabCursor int
	offset    int
	notice    string

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

// activatedMsg reports the outcome of focusing a tab in Chrome.
type activatedMsg struct {
	err error
}

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

func (m Model) activateTab() tea.Cmd {
	return func() tea.Msg {
		window := m.windows[m.winCursor]
		tab := window.Tabs[m.tabCursor]
		ctx, cancel := context.WithTimeout(m.ctx, activateTimeout)
		defer cancel()
		return activatedMsg{err: m.source.ActivateTab(ctx, window.Index, tab.Index)}
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.load(), m.spinnerTick())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m = m.clampOffset()
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
		m.winCursor = 0
		return m, nil
	case tea.MouseWheelMsg:
		if m.loading || m.err != nil || m.mode != modeTabs {
			return m, nil
		}
		switch msg.Button {
		case tea.MouseWheelUp:
			m = m.moveTabCursor(-mouseWheelLines)
		case tea.MouseWheelDown:
			m = m.moveTabCursor(mouseWheelLines)
		}
		return m, nil
	case activatedMsg:
		if msg.err != nil {
			m.notice = msg.err.Error()
		}
		return m, nil
	case tea.MouseClickMsg:
		if m.loading || m.err != nil || msg.Button != tea.MouseLeft {
			return m, nil
		}
		return m.clickRow(msg.Y), nil
	case tea.KeyPressMsg:
		return m.keyPress(msg.String())
	}
	return m, nil
}

func (m Model) keyPress(key string) (tea.Model, tea.Cmd) {
	m.notice = ""
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "h", "backspace":
		if m.mode == modeTabs {
			m.mode = modeWindows
			return m, nil
		}
		if key == "esc" {
			return m, tea.Quit
		}
		return m, nil
	}
	if m.loading || m.err != nil {
		return m, nil
	}
	if m.mode == modeWindows {
		return m.keyPressWindows(key)
	}
	return m.keyPressTabs(key)
}

func (m Model) keyPressWindows(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "j", "down":
		m.winCursor = min(m.winCursor+1, len(m.windows)-1)
	case "k", "up":
		m.winCursor = max(m.winCursor-1, 0)
	case "g":
		m.winCursor = 0
	case "G":
		m.winCursor = len(m.windows) - 1
	case "enter", "l", "right":
		if len(m.windows) > 0 {
			m.mode = modeTabs
			m.tabCursor = 0
			m.offset = 0
		}
	}
	return m, nil
}

func (m Model) keyPressTabs(key string) (tea.Model, tea.Cmd) {
	tabs := m.currentTabs()
	switch key {
	case "j", "down":
		m = m.moveTabCursor(1)
	case "k", "up":
		m = m.moveTabCursor(-1)
	case "g":
		m.tabCursor = 0
		m.offset = 0
	case "G":
		m.tabCursor = len(tabs) - 1
		m.offset = max(0, len(tabs)-m.tabViewHeight())
	case "ctrl+d":
		m = m.moveTabCursor(max(1, m.tabViewHeight()/2))
	case "ctrl+u":
		m = m.moveTabCursor(-max(1, m.tabViewHeight()/2))
	case "o":
		return m, m.activateTab()
	}
	return m, nil
}

func (m Model) currentTabs() []chrome.Tab {
	if m.winCursor >= len(m.windows) {
		return nil
	}
	return m.windows[m.winCursor].Tabs
}

// clickRow handles a left click at terminal row y. Both views start their
// rows two lines down: the header and a blank line. Clicking a window drills
// into it; clicking a tab moves the cursor to it.
func (m Model) clickRow(y int) Model {
	const rowTop = 2
	if y < rowTop {
		return m
	}
	row := y - rowTop
	if m.mode == modeWindows {
		if row < len(m.windows) {
			m.winCursor = row
			m.mode = modeTabs
			m.tabCursor = 0
			m.offset = 0
		}
		return m
	}
	tabs := m.currentTabs()
	if target := m.offset + row; target < len(tabs) {
		m.tabCursor = target
	}
	return m
}

// moveTabCursor returns the model with the tab cursor moved by delta rows,
// kept inside the list, with the view scrolled so the cursor stays visible.
func (m Model) moveTabCursor(delta int) Model {
	tabs := m.currentTabs()
	if len(tabs) == 0 {
		return m
	}
	m.tabCursor = min(max(m.tabCursor+delta, 0), len(tabs)-1)
	visible := m.tabViewHeight()
	if m.tabCursor < m.offset {
		m.offset = m.tabCursor
	}
	if m.tabCursor >= m.offset+visible {
		m.offset = m.tabCursor - visible + 1
	}
	return m
}

// tabViewHeight is how many tab rows fit under the header and footer.
func (m Model) tabViewHeight() int {
	return max(1, m.height-4)
}

func (m Model) clampOffset() Model {
	if m.mode != modeTabs {
		return m
	}
	tabs := m.currentTabs()
	if len(tabs) == 0 {
		m.offset = 0
		return m
	}
	m.offset = min(max(m.offset, 0), max(0, len(tabs)-m.tabViewHeight()))
	m.tabCursor = min(max(m.tabCursor, 0), len(tabs)-1)
	visible := m.tabViewHeight()
	if m.tabCursor < m.offset {
		m.offset = m.tabCursor
	}
	if m.tabCursor >= m.offset+visible {
		m.offset = m.tabCursor - visible + 1
	}
	return m
}

func (m Model) View() tea.View {
	var content string
	switch {
	case m.err != nil:
		content = m.viewError()
	case m.loading:
		content = m.viewLoading()
	case m.mode == modeTabs:
		content = m.viewTabs()
	default:
		content = m.viewWindows()
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m Model) viewLoading() string {
	return fmt.Sprintf("%s %s\n", spinnerFrames[m.spinnerFrame], dimStyle.Render("asking Chrome for its tabs…"))
}

func (m Model) viewError() string {
	return badStyle.Render(fmt.Sprintf("error: %v", m.err)) + "\n\n" +
		dimStyle.Render("press q to quit")
}

func (m Model) viewWindows() string {
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
		if i == m.winCursor {
			b.WriteString(selectedStyle.Render("▸ " + line))
		} else {
			b.WriteString("  " + line)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("enter open · j/k select · g/G ends · q quit"))
	return b.String()
}

func (m Model) viewTabs() string {
	tabs := m.currentTabs()
	window := m.windows[m.winCursor]

	var b strings.Builder
	header := titleStyle.Render(fmt.Sprintf("Window %d · %d %s", window.Index, len(tabs), plural("tab", len(tabs))))
	b.WriteString(truncate(header, m.width))
	b.WriteString("\n\n")
	visible := m.tabViewHeight()
	for i := m.offset; i < min(m.offset+visible, len(tabs)); i++ {
		tab := tabs[i]
		cursor := "  "
		if i == m.tabCursor {
			cursor = "▸ "
		}
		row := fmt.Sprintf("%s%-4d %s", cursor, tab.Index, tab.Title)
		if tab.URL != "" {
			row += "  " + dimStyle.Render(tab.URL)
		}
		if i == m.tabCursor {
			row = selectedStyle.Render(row)
		}
		b.WriteString(truncate(row, m.width))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	footer := dimStyle.Render(fmt.Sprintf(
		"%d/%d · j/k wheel ctrl+d/u scroll · g/G ends · o open · esc back · q quit",
		m.tabCursor+1, len(tabs),
	))
	if m.notice != "" {
		footer = badStyle.Render(truncate(m.notice, m.width))
	}
	b.WriteString(truncate(footer, m.width))
	return b.String()
}

func truncate(row string, width int) string {
	if width <= 0 {
		return row
	}
	return ansi.Truncate(row, width, "…")
}

func plural(word string, n int) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
