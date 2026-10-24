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
	windows     []chrome.Window
	err         error
	activateURL string
	activation  chrome.Activation
	found       bool
	activateErr error
	activated   []int
	urls        []string
}

func (f *fakeSource) ListTabs(context.Context) ([]chrome.Window, error) {
	return f.windows, f.err
}

func (f *fakeSource) ActivateTab(_ context.Context, url string, windowIndex, tabIndex int) (chrome.Activation, bool, error) {
	f.activated = append(f.activated, tabIndex)
	f.urls = append(f.urls, url)
	if f.activateErr != nil {
		return chrome.Activation{}, false, f.activateErr
	}
	if !f.found {
		return chrome.Activation{}, false, nil
	}
	if f.activation.Moved {
		f.activated[len(f.activated)-1] = f.activation.TabIndex
	}
	return f.activation, f.found, nil
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
	return newLoadedFrom(&fakeSource{windows: windows})
}

func newLoadedFrom(source *fakeSource) Model {
	m := New(context.Background(), source)
	return update(m, tabsMsg{windows: source.windows})
}

func update(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func runCmd(m Model, cmd tea.Cmd) Model {
	if cmd == nil {
		return m
	}
	return update(m, cmd())
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
	cmd := New(context.Background(), &fakeSource{}).Init()
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
	m := New(context.Background(), &fakeSource{})
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

func TestHalfPageScrollsContentWithoutMovingCursor(t *testing.T) {
	m := resize(update(newLoaded(manyTabs(30)), key(tea.KeyEnter)), 80, 10)
	m = update(m, key('G'))
	if m.tabCursor != 29 || m.offset != 24 {
		t.Fatalf("after G, tabCursor = %d, offset = %d, want 29/24", m.tabCursor, m.offset)
	}
	m = update(m, key('u', tea.ModCtrl))
	if m.offset != 21 {
		t.Errorf("after ctrl+u, offset = %d, want 21", m.offset)
	}
	if m.tabCursor != 29 {
		t.Errorf("after ctrl+u, tabCursor = %d, want 29 (cursor should not move)", m.tabCursor)
	}
	m = update(m, key('d', tea.ModCtrl))
	if m.offset != 24 {
		t.Errorf("after ctrl+d, offset = %d, want 24 (clamped to the bottom)", m.offset)
	}
	if m.tabCursor != 29 {
		t.Errorf("after ctrl+d, tabCursor = %d, want 29 (cursor should not move)", m.tabCursor)
	}
}

func TestMouseWheelScrollsContentWithoutMovingCursor(t *testing.T) {
	m := resize(update(newLoaded(manyTabs(30)), key(tea.KeyEnter)), 80, 10)
	m = update(m, key('G'))
	m = update(m, tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if m.offset != 21 {
		t.Errorf("after wheel up, offset = %d, want 21", m.offset)
	}
	if m.tabCursor != 29 {
		t.Errorf("after wheel up, tabCursor = %d, want 29 (cursor should not move)", m.tabCursor)
	}
	m = update(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if m.offset != 24 {
		t.Errorf("after wheel down, offset = %d, want 24 (clamped)", m.offset)
	}
}

func TestResizeClampsOffset(t *testing.T) {
	m := resize(update(newLoaded(manyTabs(30)), key(tea.KeyEnter)), 80, 10)
	m = update(m, key('G'))
	if m.offset != 24 {
		t.Fatalf("after G, offset = %d, want 24", m.offset)
	}
	m = resize(m, 80, 24)
	if want := max(0, 30-m.tabViewHeight()); m.offset != want {
		t.Errorf("after growing, offset = %d, want %d", m.offset, want)
	}
	m = resize(m, 80, 6)
	if want := max(0, 30-m.tabViewHeight()); m.offset > want {
		t.Errorf("after shrinking, offset = %d, want at most %d", m.offset, want)
	}
}

func TestLoadErrorKeptForDisplay(t *testing.T) {
	err := errors.New("Chrome isn't running")
	m := update(New(context.Background(), &fakeSource{}), tabsMsg{err: err})
	if m.err != err {
		t.Errorf("model error = %v, want %v", m.err, err)
	}
}

func TestOpenActivatesSelectedTabByURL(t *testing.T) {
	source := &fakeSource{windows: sampleWindows(), found: true}
	m := newLoadedFrom(source)
	m = update(m, key(tea.KeyEnter))
	m = update(m, key('j'))
	next, cmd := m.Update(key('o'))
	if cmd == nil {
		t.Fatal("o should start activating the tab in Chrome")
	}
	runCmd(next.(Model), cmd)
	if len(source.urls) != 1 || source.urls[0] != "https://charm.sh" {
		t.Errorf("ActivateTab url = %v, want https://charm.sh", source.urls)
	}
	if len(source.activated) != 1 || source.activated[0] != 2 {
		t.Errorf("ActivateTab args = %v, want tab 2", source.activated)
	}
}

func TestOpenShowsErrorNotice(t *testing.T) {
	source := &fakeSource{
		windows:     sampleWindows(),
		activateErr: errors.New("osascript failed"),
	}
	m := newLoadedFrom(source)
	m = update(m, key(tea.KeyEnter))
	next, cmd := m.Update(key('o'))
	m = runCmd(next.(Model), cmd)
	if m.notice == "" {
		t.Fatal("activation error should set a notice")
	}
	m = update(m, key('j'))
	if m.notice != "" {
		t.Errorf("notice should clear on the next keypress, got %q", m.notice)
	}
}

func runMsg(m Model, cmd tea.Cmd) (Model, tea.Cmd) {
	if cmd == nil {
		return m, nil
	}
	next, nextCmd := m.Update(cmd())
	return next.(Model), nextCmd
}

func TestOpenMissingTabRefreshesList(t *testing.T) {
	source := &fakeSource{windows: sampleWindows()}
	m := newLoadedFrom(source)
	m = update(m, key(tea.KeyEnter))
	next, cmd := m.Update(key('o'))
	m, refresh := runMsg(next.(Model), cmd)
	if m.notice == "" {
		t.Fatal("a missing tab should set a notice")
	}
	if refresh == nil {
		t.Error("a missing tab should trigger a refresh")
	}
}

func TestOpenMovedTabNotifiesAndRefreshes(t *testing.T) {
	source := &fakeSource{
		windows:    sampleWindows(),
		found:      true,
		activation: chrome.Activation{WindowIndex: 2, TabIndex: 1, Moved: true},
	}
	m := newLoadedFrom(source)
	m = update(m, key(tea.KeyEnter))
	next, cmd := m.Update(key('o'))
	m, refresh := runMsg(next.(Model), cmd)
	if m.notice == "" || !strings.Contains(m.notice, "window 2") {
		t.Fatalf("a moved tab should explain where it went, notice = %q", m.notice)
	}
	if refresh == nil {
		t.Error("a moved tab should trigger a refresh")
	}
}

func TestRefreshKeyReloadsList(t *testing.T) {
	source := &fakeSource{windows: sampleWindows()}
	m := newLoadedFrom(source)
	m = update(m, key(tea.KeyEnter))
	next, cmd := m.Update(key('r'))
	if cmd == nil {
		t.Fatal("r should start a reload")
	}
	m = runCmd(next.(Model), cmd)
	if m.notice != "" {
		t.Errorf("r should not set a notice, got %q", m.notice)
	}
}

func TestOpenIgnoredOutsideTabsView(t *testing.T) {
	source := &fakeSource{windows: sampleWindows()}
	m := newLoadedFrom(source)
	m = update(m, key('o'))
	if len(source.activated) != 0 {
		t.Errorf("o in the windows view should not activate anything, got %v", source.activated)
	}
}

func click(y int) tea.Msg {
	return tea.MouseClickMsg{Y: y, Button: tea.MouseLeft}
}

func TestClickWindowDrillsIntoItsTabs(t *testing.T) {
	m := newLoaded(nil)
	m = update(m, click(2))
	if m.mode != modeTabs || m.winCursor != 0 {
		t.Fatalf("click on window row 0: mode = %v, winCursor = %d, want tabs view of window 0", m.mode, m.winCursor)
	}
	m = update(m, key(tea.KeyEscape))
	m = update(m, click(3))
	if m.mode != modeTabs || m.winCursor != 1 {
		t.Errorf("click on window row 1: mode = %v, winCursor = %d, want tabs view of window 1", m.mode, m.winCursor)
	}
}

func TestClickOutsideWindowRowsIgnored(t *testing.T) {
	m := newLoaded(nil)
	m = update(m, click(1))
	if m.mode != modeWindows {
		t.Errorf("click on the blank line opened a window, mode = %v", m.mode)
	}
	m = update(m, click(9))
	if m.mode != modeWindows {
		t.Errorf("click below the last window opened a window, mode = %v", m.mode)
	}
}

func TestClickTabMovesCursor(t *testing.T) {
	m := resize(update(newLoaded(manyTabs(20)), key(tea.KeyEnter)), 80, 8)
	m = update(m, key('G'))
	if m.offset != 16 {
		t.Fatalf("after G, offset = %d, want 16", m.offset)
	}
	m = update(m, click(2))
	if m.tabCursor != 16 {
		t.Errorf("click on the first visible row: tabCursor = %d, want 16", m.tabCursor)
	}
	m = update(m, click(5))
	if m.tabCursor != 19 {
		t.Errorf("click on the last visible row: tabCursor = %d, want 19", m.tabCursor)
	}
	m = update(m, click(7))
	if m.tabCursor != 19 {
		t.Errorf("click below the last row should not move the cursor, tabCursor = %d", m.tabCursor)
	}
}

func typeQuery(m Model, q string) Model {
	m = update(m, key('/'))
	for _, r := range q {
		m = update(m, tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
	return m
}

func TestMatchesTab(t *testing.T) {
	tab := chrome.Tab{Title: "Go Playground", URL: "https://go.dev/play"}
	cases := []struct {
		query string
		want  bool
	}{
		{"go", true},
		{"PLAY", true},
		{"playground", true},
		{"dev", true},
		{"go playground", true},
		{"playground go", true},
		{"go rust", false},
		{"rust", false},
		{"", true},
	}
	for _, c := range cases {
		if got := matchesTab(tab, strings.Fields(strings.ToLower(c.query))); got != c.want {
			t.Errorf("matchesTab(%q) = %v, want %v", c.query, got, c.want)
		}
	}
}

func TestFilterNarrowsTabsToMatches(t *testing.T) {
	m := update(newLoaded(nil), key(tea.KeyEnter))
	m = typeQuery(m, "charm")
	if m.filtering != true {
		t.Fatal("typing should keep the model in filter input mode")
	}
	tabs := m.visibleTabs()
	if len(tabs) != 1 || tabs[0].Title != "Charm" {
		t.Errorf("query %q: visible = %+v, want only Charm", m.query, tabs)
	}
}

func TestFilterCursorResetsWhileTyping(t *testing.T) {
	m := update(newLoaded(manyTabs(20)), key(tea.KeyEnter))
	m = update(m, key('G'))
	if m.tabCursor != 19 {
		t.Fatalf("after G, tabCursor = %d, want 19", m.tabCursor)
	}
	m = typeQuery(m, "tab")
	if m.tabCursor != 0 || m.offset != 0 {
		t.Errorf("after typing, tabCursor = %d, offset = %d, want 0/0", m.tabCursor, m.offset)
	}
}

func TestEnterLeavesFilterModeKeepsQuery(t *testing.T) {
	m := typeQuery(update(newLoaded(nil), key(tea.KeyEnter)), "charm")
	m = update(m, key(tea.KeyEnter))
	if m.filtering {
		t.Error("enter should leave filter input mode")
	}
	if m.query != "charm" {
		t.Errorf("enter should keep the query, got %q", m.query)
	}
	if tabs := m.visibleTabs(); len(tabs) != 1 {
		t.Errorf("filter should stay applied, visible = %+v", tabs)
	}
}

func TestEscClearsFilterAndBacksOut(t *testing.T) {
	m := typeQuery(update(newLoaded(nil), key(tea.KeyEnter)), "charm")
	m = update(m, key(tea.KeyEscape))
	if m.query != "" || m.filtering {
		t.Errorf("esc should clear the query and leave input mode, got query %q filtering %v", m.query, m.filtering)
	}
	if tabs := m.visibleTabs(); len(tabs) != 2 {
		t.Errorf("all tabs should be back, visible = %+v", tabs)
	}
	m = typeQuery(m, "charm")
	m = update(m, key(tea.KeyEnter))
	m = update(m, key(tea.KeyEscape))
	if m.query != "" {
		t.Fatalf("first esc should clear the query, got %q", m.query)
	}
	m = update(m, key(tea.KeyEscape))
	if m.mode != modeWindows {
		t.Errorf("esc with no query should back out to the windows view, mode = %v", m.mode)
	}
}

func TestSlashReentersFilterMode(t *testing.T) {
	m := typeQuery(update(newLoaded(nil), key(tea.KeyEnter)), "charm")
	m = update(m, key(tea.KeyEnter))
	m = update(m, key('/'))
	if !m.filtering || m.query != "charm" {
		t.Errorf("slash should resume editing the query, filtering = %v query = %q", m.filtering, m.query)
	}
}

func TestBackspaceEditsQuery(t *testing.T) {
	m := typeQuery(update(newLoaded(nil), key(tea.KeyEnter)), "charm")
	m = update(m, key(tea.KeyBackspace))
	if m.query != "char" {
		t.Errorf("after backspace, query = %q, want %q", m.query, "char")
	}
}

func TestOpenUsesFilteredTabIndices(t *testing.T) {
	source := &fakeSource{windows: sampleWindows()}
	m := newLoadedFrom(source)
	m = update(m, key(tea.KeyEnter))
	m = typeQuery(m, "charm")
	m = update(m, key(tea.KeyEnter))
	next, cmd := m.Update(key('o'))
	if cmd == nil {
		t.Fatal("o should start activating the filtered tab in Chrome")
	}
	runCmd(next.(Model), cmd)
	if len(source.activated) != 1 || source.activated[0] != 2 {
		t.Errorf("ActivateTab args = %v, want window 1's tab 2 via the filter", source.activated)
	}
	if len(source.urls) != 1 || source.urls[0] != "https://charm.sh" {
		t.Errorf("ActivateTab url = %v, want https://charm.sh", source.urls)
	}
}

func TestFilterLeavesBackOutToWindows(t *testing.T) {
	m := typeQuery(update(newLoaded(nil), key(tea.KeyEnter)), "charm")
	m = update(m, key(tea.KeyEnter))
	m = update(m, key('h'))
	if m.mode != modeWindows {
		t.Errorf("h should back out of the tabs view, mode = %v", m.mode)
	}
	if m.query != "" {
		t.Errorf("backing out should clear the query, got %q", m.query)
	}
}

func TestNoticeExpiresAfterTimeout(t *testing.T) {
	source := &fakeSource{
		windows:    sampleWindows(),
		found:      true,
		activation: chrome.Activation{WindowIndex: 2, TabIndex: 1, Moved: true},
	}
	m := newLoadedFrom(source)
	m = update(m, key(tea.KeyEnter))
	next, cmd := m.Update(key('o'))
	m, _ = runMsg(next.(Model), cmd)
	if m.notice == "" {
		t.Fatal("a moved tab should set a notice")
	}
	m = update(m, noticeExpiredMsg{gen: m.noticeGen})
	if m.notice != "" {
		t.Errorf("an expired notice should clear itself, got %q", m.notice)
	}
}

func TestExpiredNoticeDoesNotClearNewerOne(t *testing.T) {
	source := &fakeSource{
		windows:    sampleWindows(),
		found:      true,
		activation: chrome.Activation{WindowIndex: 2, TabIndex: 1, Moved: true},
	}
	m := newLoadedFrom(source)
	m = update(m, key(tea.KeyEnter))
	next, cmd := m.Update(key('o'))
	m, _ = runMsg(next.(Model), cmd)
	staleGen := m.noticeGen - 1
	m2, _ := m.setNotice("newer notice")
	m2 = update(m2, noticeExpiredMsg{gen: staleGen})
	if m2.notice != "newer notice" {
		t.Errorf("an old timer should not clear a newer notice, got %q", m2.notice)
	}
}
