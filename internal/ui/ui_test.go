package ui

import (
	"context"
	"errors"
	"strings"
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

func newLoaded() Model {
	m := New(context.Background(), fakeSource{windows: sampleWindows()})
	return update(m, tabsMsg{windows: sampleWindows()})
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

func TestInitLoadsTabsAndStartsSpinner(t *testing.T) {
	cmd := New(context.Background(), fakeSource{}).Init()
	if cmd == nil {
		t.Fatal("Init returned no command, want a batched load and spinner tick")
	}
}

func TestLoadingViewShowsPrompt(t *testing.T) {
	view := New(context.Background(), fakeSource{}).View().Content
	if !strings.Contains(view, "asking Chrome for its tabs") {
		t.Errorf("loading view should mention what it's doing:\n%s", view)
	}
}

func TestLoadedViewListsWindowsWithTabCounts(t *testing.T) {
	view := newLoaded().View().Content
	for _, want := range []string{"Google Chrome: 2 windows, 3 tabs", "Window 1", "Window 2", "2 tabs", "1 tab"} {
		if !strings.Contains(view, want) {
			t.Errorf("view should contain %q:\n%s", want, view)
		}
	}
}

func TestErrorViewShowsMessage(t *testing.T) {
	m := New(context.Background(), fakeSource{err: errors.New("Chrome isn't running")})
	m = update(m, tabsMsg{err: errors.New("Chrome isn't running")})
	if !strings.Contains(m.View().Content, "error: Chrome isn't running") {
		t.Errorf("error view should show the error:\n%s", m.View().Content)
	}
}

func TestQuitKeyReturnsQuitCommand(t *testing.T) {
	quits := []tea.Msg{
		key('q'),
		key(tea.KeyEscape),
		key('c', tea.ModCtrl),
	}
	for _, msg := range quits {
		if _, cmd := newLoaded().Update(msg); cmd == nil {
			t.Errorf("%v should quit", msg)
		}
	}
}

func TestSelectionMovesAndClamps(t *testing.T) {
	m := newLoaded()
	m = update(m, key('j'))
	if m.selected != 1 {
		t.Errorf("after j, selected = %d, want 1", m.selected)
	}
	m = update(m, key('j'))
	if m.selected != 1 {
		t.Errorf("after j past the end, selected = %d, want 1", m.selected)
	}
	m = update(m, key('k'))
	if m.selected != 0 {
		t.Errorf("after k, selected = %d, want 0", m.selected)
	}
	m = update(m, key('k'))
	if m.selected != 0 {
		t.Errorf("after k past the top, selected = %d, want 0", m.selected)
	}
}

func TestSelectionJumpsToTopAndBottom(t *testing.T) {
	m := newLoaded()
	m = update(m, key('G'))
	if m.selected != 1 {
		t.Errorf("after G, selected = %d, want 1", m.selected)
	}
	m = update(m, key('g'))
	if m.selected != 0 {
		t.Errorf("after g, selected = %d, want 0", m.selected)
	}
}

func TestSelectedRowIsHighlighted(t *testing.T) {
	loaded := newLoaded()
	moved := update(loaded, key('j'))
	if moved.View().Content == loaded.View().Content {
		t.Error("moving the selection should change the view")
	}
}

func TestKeysIgnoredWhileLoading(t *testing.T) {
	m := New(context.Background(), fakeSource{})
	next := update(m, key('j'))
	if next.selected != m.selected {
		t.Errorf("j while loading moved the selection to %d, want no movement", next.selected)
	}
}

func TestSpinnerStopsAfterLoad(t *testing.T) {
	m := newLoaded()
	m = update(m, spinnerTickMsg{})
	if m.spinnerFrame != 0 {
		t.Errorf("spinner advanced after load, frame = %d, want 0", m.spinnerFrame)
	}
}
