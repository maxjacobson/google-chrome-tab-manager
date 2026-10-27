// Package chrome reads the open windows and tabs from Google Chrome.
package chrome

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// Tab is one open tab: its position in the window, its URL, and its title.
type Tab struct {
	Index int
	URL   string
	Title string
}

// Window is one Chrome window and the tabs it holds, in order.
type Window struct {
	Index int
	Tabs  []Tab
}

// Chrome lists tabs by asking Chrome through AppleScript. The zero value works.
type Chrome struct{}

// listTabsScript produces one TSV line per tab: window, tab, URL, title.
// It reads each window's URLs and titles in bulk; asking tab-by-tab costs
// two Apple Events per tab, which takes ~45s for a thousand-tab window.
const listTabsScript = `
tell application "Google Chrome"
    set output to {}
    repeat with w from 1 to count of windows
        try
            set tabUrls to URL of tabs of window w
            set tabTitles to title of tabs of window w
            repeat with t from 1 to count of tabUrls
                set tabUrl to item t of tabUrls
                set tabTitle to item t of tabTitles
                if tabUrl is missing value then set tabUrl to ""
                if tabTitle is missing value then set tabTitle to ""
                set end of output to (w as text) & "\t" & (t as text) & "\t" & tabUrl & "\t" & tabTitle
            end repeat
        end try
    end repeat
    set astid to AppleScript's text item delimiters
    set AppleScript's text item delimiters to linefeed
    set output to output as text
    set AppleScript's text item delimiters to astid
    return output
end tell
`

// ListTabs returns Chrome's open windows and their tabs. It only works on
// macOS, and it requires Chrome to already be running (asking a closed
// Chrome for its tabs would silently launch it).
func (Chrome) ListTabs(ctx context.Context) ([]Window, error) {
	if runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("listing Chrome tabs only works on macOS")
	}
	if err := exec.CommandContext(ctx, "pgrep", "-x", "Google Chrome").Run(); err != nil {
		return nil, fmt.Errorf("Chrome isn't running")
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "osascript", "-e", listTabsScript)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("running osascript: %w: %s", err, msg)
		}
		return nil, fmt.Errorf("running osascript: %w", err)
	}
	return ParseTabs(stdout.String())
}

// Activation reports where a tab ended up being focused in Chrome.
type Activation struct {
	WindowIndex int
	TabIndex    int
	Moved       bool
}

// activateTabScript focuses the tab with targetURL. Chrome renumbers
// windows (by front-to-back order) and tabs (as they open and close), so
// indices go stale; the remembered position is a fast path, and the scan
// afterwards finds the tab by URL when the position no longer matches. It
// answers "ok <window> <tab>", "moved <window> <tab>", or "missing".
const activateTabScript = `
tell application "Google Chrome"
    activate
    set targetURL to "%s"
    try
        if URL of tab %d of window %d is targetURL then
            set active tab index of window %d to %d
            set index of window %d to 1
            return "ok " & %d & " " & %d
        end if
    end try
    repeat with w from 1 to count of windows
        try
            set tabUrls to URL of tabs of window w
            repeat with t from 1 to count of tabUrls
                if item t of tabUrls is targetURL then
                    set active tab index of window w to t
                    set index of window w to 1
                    return "moved " & w & " " & t
                end if
            end repeat
        end try
    end repeat
    return "missing"
end tell
`

// ActivateTab brings Chrome to the front with the tab whose URL matches,
// raising its window. It trusts the remembered window and tab indices only
// if the URL there still matches; otherwise it finds the tab by scanning
// each window's URLs. found is false when no open tab has the URL.
func (Chrome) ActivateTab(ctx context.Context, url string, windowIndex, tabIndex int) (Activation, bool, error) {
	if runtime.GOOS != "darwin" {
		return Activation{}, false, fmt.Errorf("activating a Chrome tab only works on macOS")
	}
	script := fmt.Sprintf(activateTabScript, escapeAppleScript(url), windowIndex, tabIndex, windowIndex, tabIndex, windowIndex, windowIndex, tabIndex)
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return Activation{}, false, fmt.Errorf("running osascript: %w: %s", err, msg)
		}
		return Activation{}, false, fmt.Errorf("running osascript: %w", err)
	}
	return ParseActivation(strings.TrimSpace(stdout.String()))
}

// ParseActivation parses activateTabScript's answer.
func ParseActivation(output string) (Activation, bool, error) {
	fields := strings.Fields(output)
	if len(fields) == 0 {
		return Activation{}, false, fmt.Errorf("unexpected activation output %q", output)
	}
	switch fields[0] {
	case "ok", "moved":
		if len(fields) != 3 {
			return Activation{}, false, fmt.Errorf("unexpected activation output %q", output)
		}
		windowIndex, err := strconv.Atoi(fields[1])
		if err != nil {
			return Activation{}, false, fmt.Errorf("parsing window index in %q: %w", output, err)
		}
		tabIndex, err := strconv.Atoi(fields[2])
		if err != nil {
			return Activation{}, false, fmt.Errorf("parsing tab index in %q: %w", output, err)
		}
		return Activation{WindowIndex: windowIndex, TabIndex: tabIndex, Moved: fields[0] == "moved"}, true, nil
	case "missing":
		return Activation{}, false, nil
	}
	return Activation{}, false, fmt.Errorf("unexpected activation output %q", output)
}

// closeTabsScript closes every tab whose URL is in the target list and
// reports how many it closed. Chrome renumbers tabs as they close, so it
// visits each window's tabs from last to first: closing tab t only shifts
// tabs after t, which were already visited.
const closeTabsScript = `
tell application "Google Chrome"
    set targets to {%s}
    set closed to 0
    repeat with w from 1 to count of windows
        try
            set tabUrls to URL of tabs of window w
            repeat with t from count of tabUrls to 1 by -1
                if item t of tabUrls is in targets then
                    close tab t of window w
                    set closed to closed + 1
                end if
            end repeat
        end try
    end repeat
    return closed as text
end tell
`

// CloseTabs closes every open tab whose URL is in urls, across all windows,
// and returns how many tabs it closed. Tabs are matched by URL because
// Chrome's window and tab numbering shifts as tabs close; a URL that is open
// more than once is closed in every window that holds it.
func (Chrome) CloseTabs(ctx context.Context, urls []string) (int, error) {
	if runtime.GOOS != "darwin" {
		return 0, fmt.Errorf("closing Chrome tabs only works on macOS")
	}
	if len(urls) == 0 {
		return 0, nil
	}
	script := fmt.Sprintf(closeTabsScript, appleScriptList(urls))
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return 0, fmt.Errorf("running osascript: %w: %s", err, msg)
		}
		return 0, fmt.Errorf("running osascript: %w", err)
	}
	return ParseCloseCount(strings.TrimSpace(stdout.String()))
}

// ParseCloseCount parses closeTabsScript's answer: the number of closed tabs.
func ParseCloseCount(output string) (int, error) {
	count, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil {
		return 0, fmt.Errorf("unexpected close output %q: %w", output, err)
	}
	return count, nil
}

// appleScriptList renders urls as an AppleScript list literal.
func appleScriptList(urls []string) string {
	quoted := make([]string, len(urls))
	for i, url := range urls {
		quoted[i] = `"` + escapeAppleScript(url) + `"`
	}
	return strings.Join(quoted, ", ")
}

func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	return strings.ReplaceAll(s, "\"", "\\\"")
}

// ParseTabs parses the TSV output of listTabsScript into ordered windows.
// Each line is "window<TAB>tab<TAB>url<TAB>title"; a title may itself
// contain tab characters, which are rejoined rather than treated as fields.
func ParseTabs(output string) ([]Window, error) {
	var windows []Window
	positions := map[int]int{}
	output = strings.ReplaceAll(output, "\r\n", "\n")
	output = strings.ReplaceAll(output, "\r", "\n")
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 4 {
			return nil, fmt.Errorf("unexpected line %q", line)
		}
		windowIndex, err := strconv.Atoi(fields[0])
		if err != nil {
			return nil, fmt.Errorf("parsing window index in %q: %w", line, err)
		}
		tabIndex, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, fmt.Errorf("parsing tab index in %q: %w", line, err)
		}
		tab := Tab{
			Index: tabIndex,
			URL:   fields[2],
			Title: strings.Join(fields[3:], "\t"),
		}
		if pos, ok := positions[windowIndex]; ok {
			windows[pos].Tabs = append(windows[pos].Tabs, tab)
			continue
		}
		positions[windowIndex] = len(windows)
		windows = append(windows, Window{Index: windowIndex, Tabs: []Tab{tab}})
	}
	return windows, nil
}
