package ui

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/maxjacobson/google-chrome-tab-manager/internal/chrome"
)

type fakeSource struct {
	windows []chrome.Window
	err     error
}

func (f fakeSource) ListTabs(context.Context) ([]chrome.Window, error) {
	return f.windows, f.err
}

func sampleWindows() []chrome.Window {
	return []chrome.Window{
		{Index: 1, Tabs: []chrome.Tab{
			{Index: 1, URL: "https://go.dev", Title: "Go"},
			{Index: 2, URL: "https://charm.sh", Title: "Charm"},
		}},
		{Index: 2, Tabs: []chrome.Tab{
			{Index: 1, URL: "https://example.com", Title: "Example Domain"},
		}},
	}
}

func manyTabs(n int) []chrome.Window {
	tabs := make([]chrome.Tab, n)
	for i := range tabs {
		tabs[i] = chrome.Tab{Index: i + 1, URL: "https://example.com", Title: "Tab"}
	}
	return []chrome.Window{{Index: 1, Tabs: tabs}}
}

func newLoaded(windows []chrome.Window) Model {
	if windows == nil {
		windows = sampleWindows()
	}
	m := New(context.Background(), fakeSource{windows: windows})
	return update(m, tabsMsg{windows: windows})
}

func update(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func key(code rune, mod ...tea.KeyMod) tea.Msg {
	k := tea.Key{Code: code}
	for _, m := range mod {
		k.Mod |= m
	}
	return tea.KeyPressMsg(k)
}

func resize(m Model, w, h int) Model {
	return update(m, tea.WindowSizeMsg{Width: w, Height: h})
}

func TestInitLoadsTabsAndStartsSpinner(t *testing.T) {
	cmd := New(context.Background(), fakeSource{}).Init()
	if cmd == nil {
		t.Fatal("Init returned no command, want a batched load and spinner tick")
	}
}

func TestQuitKeyReturnsQuitCommand(t *testing.T) {
	quits := []tea.Msg{
		key('q'),
		key(tea.KeyEscape),
		key('c', tea.ModCtrl),
	}
	for _, msg := range quits {
		if _, cmd := newLoaded(nil).Update(msg); cmd == nil {
			t.Errorf("%v should quit", msg)
		}
	}
}

func TestWindowSelectionMovesAndClamps(t *testing.T) {
	m := newLoaded(nil)
	m = update(m, key('j'))
	if m.winCursor != 1 {
		t.Errorf("after j, winCursor = %d, want 1", m.winCursor)
	}
	m = update(m, key('j'))
	if m.winCursor != 1 {
		t.Errorf("after j past the end, winCursor = %d, want 1", m.winCursor)
	}
	m = update(m, key('k'))
	if m.winCursor != 0 {
		t.Errorf("after k, winCursor = %d, want 0", m.winCursor)
	}
	m = update(m, key('k'))
	if m.winCursor != 0 {
		t.Errorf("after k past the top, winCursor = %d, want 0", m.winCursor)
	}
}

func TestWindowSelectionJumpsToTopAndBottom(t *testing.T) {
	m := newLoaded(nil)
	m = update(m, key('G'))
	if m.winCursor != 1 {
		t.Errorf("after G, winCursor = %d, want 1", m.winCursor)
	}
	m = update(m, key('g'))
	if m.winCursor != 0 {
		t.Errorf("after g, winCursor = %d, want 0", m.winCursor)
	}
}

func TestKeysIgnoredWhileLoading(t *testing.T) {
	m := New(context.Background(), fakeSource{})
	next := update(m, key('j'))
	if next.winCursor != m.winCursor {
		t.Errorf("j while loading moved the selection to %d, want no movement", next.winCursor)
	}
}

func TestSpinnerStopsAfterLoad(t *testing.T) {
	m := newLoaded(nil)
	m = update(m, spinnerTickMsg{})
	if m.spinnerFrame != 0 {
		t.Errorf("spinner advanced after load, frame = %d, want 0", m.spinnerFrame)
	}
}

func TestEnterOpensSelectedWindow(t *testing.T) {
	m := update(newLoaded(nil), key(tea.KeyEnter))
	if m.mode != modeTabs {
		t.Errorf("after enter, mode = %v, want modeTabs", m.mode)
	}
	if m.tabCursor != 0 || m.offset != 0 {
		t.Errorf("after enter, tabCursor = %d, offset = %d, want 0/0", m.tabCursor, m.offset)
	}
}

func TestEscBacksOutOfTabsView(t *testing.T) {
	for _, msg := range []tea.Msg{key(tea.KeyEscape), key(tea.KeyBackspace), key('h')} {
		m := update(newLoaded(nil), key(tea.KeyEnter))
		if m = update(m, msg); m.mode != modeWindows {
			t.Errorf("%v should back out of the tabs view, mode = %v", msg, m.mode)
		}
	}
}

func TestTabCursorMovesAndClamps(t *testing.T) {
	m := update(newLoaded(nil), key(tea.KeyEnter))
	m = update(m, key('j'))
	if m.tabCursor != 1 {
		t.Errorf("after j, tabCursor = %d, want 1", m.tabCursor)
	}
	m = update(m, key('j'))
	if m.tabCursor != 1 {
		t.Errorf("after j past the end, tabCursor = %d, want 1", m.tabCursor)
	}
	m = update(m, key('k'))
	if m.tabCursor != 0 {
		t.Errorf("after k, tabCursor = %d, want 0", m.tabCursor)
	}
}

func TestTabCursorJumpsToTopAndBottom(t *testing.T) {
	m := resize(update(newLoaded(manyTabs(20)), key(tea.KeyEnter)), 80, 8)
	m = update(m, key('G'))
	if m.tabCursor != 19 {
		t.Errorf("after G, tabCursor = %d, want 19", m.tabCursor)
	}
	m = update(m, key('g'))
	if m.tabCursor != 0 || m.offset != 0 {
		t.Errorf("after g, tabCursor = %d, offset = %d, want 0/0", m.tabCursor, m.offset)
	}
}

func TestViewScrollsToKeepCursorVisible(t *testing.T) {
	m := resize(update(newLoaded(manyTabs(20)), key(tea.KeyEnter)), 80, 8)
	m = update(m, key('G'))
	if m.offset != 16 {
		t.Errorf("after G, offset = %d, want 16", m.offset)
	}
	m = update(m, key('g'))
	if m.offset != 0 {
		t.Errorf("after g, offset = %d, want 0", m.offset)
	}
}

func TestHalfPageScrolling(t *testing.T) {
	m := resize(update(newLoaded(manyTabs(30)), key(tea.KeyEnter)), 80, 10)
	m = update(m, key('G'))
	if m.tabCursor != 29 {
		t.Fatalf("after G, tabCursor = %d, want 29", m.tabCursor)
	}
	m = update(m, key('u', tea.ModCtrl))
	if m.tabCursor != 26 {
		t.Errorf("after ctrl+u, tabCursor = %d, want 26", m.tabCursor)
	}
	m = update(m, key('d', tea.ModCtrl))
	if m.tabCursor != 29 {
		t.Errorf("after ctrl+d, tabCursor = %d, want 29", m.tabCursor)
	}
}

func TestMouseWheelScrolls(t *testing.T) {
	m := resize(update(newLoaded(manyTabs(30)), key(tea.KeyEnter)), 80, 10)
	m = update(m, key('G'))
	m = update(m, tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if m.tabCursor != 26 {
		t.Errorf("after wheel up, tabCursor = %d, want 26", m.tabCursor)
	}
	m = update(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if m.tabCursor != 29 {
		t.Errorf("after wheel down, tabCursor = %d, want 29", m.tabCursor)
	}
}

func TestResizeKeepsCursorVisible(t *testing.T) {
	m := resize(update(newLoaded(manyTabs(30)), key(tea.KeyEnter)), 80, 10)
	m = update(m, key('G'))
	if m.offset != 24 {
		t.Fatalf("after G, offset = %d, want 24", m.offset)
	}
	m = resize(m, 80, 24)
	visible := m.tabViewHeight()
	if m.tabCursor < m.offset || m.tabCursor >= m.offset+visible {
		t.Errorf("cursor %d outside view [%d, %d)", m.tabCursor, m.offset, m.offset+visible)
	}
	m = resize(m, 80, 6)
	visible = m.tabViewHeight()
	if m.tabCursor < m.offset || m.tabCursor >= m.offset+visible {
		t.Errorf("after shrinking, cursor %d outside view [%d, %d)", m.tabCursor, m.offset, m.offset+visible)
	}
}

func TestLoadErrorKeptForDisplay(t *testing.T) {
	err := errors.New("Chrome isn't running")
	m := update(New(context.Background(), fakeSource{}), tabsMsg{err: err})
	if m.err != err {
		t.Errorf("model error = %v, want %v", m.err, err)
	}
}
