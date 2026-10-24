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
