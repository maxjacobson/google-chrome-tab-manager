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
	// noticeTimeout is how long a footer notice sticks around on its own.
	noticeTimeout = 5 * time.Second
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
	ActivateTab(ctx context.Context, url string, windowIndex, tabIndex int) (chrome.Activation, bool, error)
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
	// noticeGen counts notices so an expiring timer only clears the notice
	// it was armed for, never a newer one.
	noticeGen int

	// filtering is true while the user is typing a query; the query stays
	// applied (and the list filtered) after enter leaves input mode.
	filtering bool
	query     string

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

// activatedMsg reports the outcome of focusing a tab in Chrome: where it
// was found (if anywhere), and any error.
type activatedMsg struct {
	activation chrome.Activation
	found      bool
	err        error
}

// noticeExpiredMsg clears the footer notice five seconds after it was set.
type noticeExpiredMsg struct {
	gen int
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

// setNotice shows a message in the footer and returns a command that clears
// it after noticeTimeout.
func (m Model) setNotice(text string) (Model, tea.Cmd) {
	m.notice = text
	m.noticeGen++
	gen := m.noticeGen
	return m, tea.Tick(noticeTimeout, func(time.Time) tea.Msg { return noticeExpiredMsg{gen: gen} })
}

func (m Model) activateTab() tea.Cmd {
	return func() tea.Msg {
		window := m.windows[m.winCursor]
		tab := m.visibleTabs()[m.tabCursor]
		ctx, cancel := context.WithTimeout(m.ctx, activateTimeout)
		defer cancel()
		activation, found, err := m.source.ActivateTab(ctx, tab.URL, window.Index, tab.Index)
		return activatedMsg{activation: activation, found: found, err: err}
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
		m.tabCursor = 0
		m.offset = 0
		return m, nil
	case tea.MouseWheelMsg:
		if m.loading || m.err != nil || m.mode != modeTabs {
			return m, nil
		}
		switch msg.Button {
		case tea.MouseWheelUp:
			m = m.scrollTabs(-mouseWheelLines)
		case tea.MouseWheelDown:
			m = m.scrollTabs(mouseWheelLines)
		}
		return m, nil
	case activatedMsg:
		switch {
		case msg.err != nil:
			return m.setNotice(msg.err.Error())
		case !msg.found:
			m, notice := m.setNotice("that tab is no longer open; refreshing")
			return m, tea.Batch(notice, m.load())
		case msg.activation.Moved:
			m, notice := m.setNotice(fmt.Sprintf(
				"that tab is now window %d, tab %d; refreshing",
				msg.activation.WindowIndex, msg.activation.TabIndex,
			))
			return m, tea.Batch(notice, m.load())
		}
		return m, nil
	case noticeExpiredMsg:
		if msg.gen == m.noticeGen {
			m.notice = ""
		}
		return m, nil
	case tea.MouseClickMsg:
		if m.loading || m.err != nil || msg.Button != tea.MouseLeft {
			return m, nil
		}
		return m.clickRow(msg.Y), nil
	case tea.KeyPressMsg:
		if m.filtering {
			return m.filterInput(msg)
		}
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
			if key == "esc" && m.query != "" {
				m = m.setQuery("")
				return m, nil
			}
			m.mode = modeWindows
			m = m.setQuery("")
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
	switch key {
	case "r":
		m.loading = true
		return m, m.load()
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
	tabs := m.visibleTabs()
	switch key {
	case "j", "down":
		m = m.moveTabCursor(1)
	case "k", "up":
		m = m.moveTabCursor(-1)
	case "g":
		m.tabCursor = 0
		m.offset = 0
	case "G":
		if len(tabs) > 0 {
			m.tabCursor = len(tabs) - 1
			m.offset = max(0, len(tabs)-m.tabViewHeight())
		}
	case "ctrl+d":
		m = m.scrollTabs(max(1, m.tabViewHeight()/2))
	case "ctrl+u":
		m = m.scrollTabs(-max(1, m.tabViewHeight()/2))
	case "o":
		if m.tabCursor < len(tabs) {
			return m, m.activateTab()
		}
	case "/":
		m.filtering = true
	}
	return m, nil
}

func (m Model) currentTabs() []chrome.Tab {
	if m.winCursor >= len(m.windows) {
		return nil
	}
	return m.windows[m.winCursor].Tabs
}

// visibleTabs is the window's tabs with the current query applied: every
// whitespace-separated term must appear in the title or the URL.
func (m Model) visibleTabs() []chrome.Tab {
	tabs := m.currentTabs()
	if m.query == "" {
		return tabs
	}
	terms := strings.Fields(strings.ToLower(m.query))
	var out []chrome.Tab
	for _, tab := range tabs {
		if matchesTab(tab, terms) {
			out = append(out, tab)
		}
	}
	return out
}

func matchesTab(tab chrome.Tab, terms []string) bool {
	title := strings.ToLower(tab.Title)
	url := strings.ToLower(tab.URL)
	for _, term := range terms {
		if !strings.Contains(title, term) && !strings.Contains(url, term) {
			return false
		}
	}
	return true
}

// setQuery applies a new filter query, moving the cursor back to the top of
// the filtered list.
func (m Model) setQuery(q string) Model {
	m.query = q
	m.tabCursor = 0
	m.offset = 0
	return m
}

// filterInput handles a keypress while the user is typing a query: runing
// keys build the query, enter accepts it, and esc clears it.
func (m Model) filterInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "enter":
		m.filtering = false
	case "esc":
		m.filtering = false
		m = m.setQuery("")
	case "backspace":
		if runes := []rune(m.query); len(runes) > 0 {
			m = m.setQuery(string(runes[:len(runes)-1]))
		}
	case "space":
		m = m.setQuery(m.query + " ")
	default:
		if msg.Text != "" {
			m = m.setQuery(m.query + msg.Text)
		}
	}
	return m, nil
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
	tabs := m.visibleTabs()
	if target := m.offset + row; target < len(tabs) {
		m.tabCursor = target
	}
	return m
}

// moveTabCursor returns the model with the tab cursor moved by delta rows,
// kept inside the list, with the view scrolled to bring the cursor back
// into view if it left it.
func (m Model) moveTabCursor(delta int) Model {
	tabs := m.visibleTabs()
	if len(tabs) == 0 {
		return m
	}
	m.tabCursor = min(max(m.tabCursor+delta, 0), len(tabs)-1)
	m = m.bringCursorIntoView()
	return m
}

// scrollTabs moves the list by delta rows without moving the cursor: the
// selected tab stays selected while the content slides under it.
func (m Model) scrollTabs(delta int) Model {
	tabs := m.visibleTabs()
	if len(tabs) == 0 {
		return m
	}
	m.offset = min(max(m.offset+delta, 0), max(0, len(tabs)-m.tabViewHeight()))
	return m
}

// bringCursorInView scrolls the view the minimum distance needed so the
// cursor is on screen again.
func (m Model) bringCursorIntoView() Model {
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
	tabs := m.visibleTabs()
	if len(tabs) == 0 {
		m.offset = 0
		return m
	}
	m.offset = min(max(m.offset, 0), max(0, len(tabs)-m.tabViewHeight()))
	m.tabCursor = min(max(m.tabCursor, 0), len(tabs)-1)
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
	tabs := m.visibleTabs()
	window := m.windows[m.winCursor]

	var b strings.Builder
	header := fmt.Sprintf("Window %d · %d %s", window.Index, len(tabs), plural("tab", len(tabs)))
	if m.query != "" {
		header += fmt.Sprintf(" of %d", len(m.currentTabs()))
	}
	b.WriteString(truncate(titleStyle.Render(header), m.width))
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
	if len(tabs) == 0 {
		b.WriteString(dimStyle.Render("no matching tabs"))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	if m.filtering {
		prompt := "/" + m.query + "▏"
		b.WriteString(truncate(prompt, m.width))
		return b.String()
	}
	footer := dimStyle.Render(fmt.Sprintf(
		"%d/%d · j/k wheel ctrl+d/u scroll · g/G ends · / filter · o open · esc back · q quit",
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
