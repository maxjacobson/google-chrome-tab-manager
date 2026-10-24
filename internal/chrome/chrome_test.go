package chrome

import (
	"strings"
	"testing"
)

func TestParseTabsGroupsTabsIntoWindows(t *testing.T) {
	input := strings.Join([]string{
		"1\t1\thttps://go.dev\tGo",
		"1\t2\thttps://charm.sh\tCharm",
		"2\t1\thttps://example.com\tExample Domain",
		"",
	}, "\n")

	windows, err := ParseTabs(input)
	if err != nil {
		t.Fatalf("ParseTabs: %v", err)
	}
	if len(windows) != 2 {
		t.Fatalf("got %d windows, want 2: %+v", len(windows), windows)
	}
	if windows[0].Index != 1 || len(windows[0].Tabs) != 2 {
		t.Errorf("window 1 = %+v, want index 1 with 2 tabs", windows[0])
	}
	if windows[1].Index != 2 || len(windows[1].Tabs) != 1 {
		t.Errorf("window 2 = %+v, want index 2 with 1 tab", windows[1])
	}
	if windows[0].Tabs[1].URL != "https://charm.sh" || windows[0].Tabs[1].Title != "Charm" {
		t.Errorf("second tab = %+v, want charm.sh / Charm", windows[0].Tabs[1])
	}
}

func TestParseTabsHandlesCarriageReturnsAndBlankURLs(t *testing.T) {
	input := "1\t1\t\tNo URL\r\n1\t2\thttps://go.dev\tGo\r\n"

	windows, err := ParseTabs(input)
	if err != nil {
		t.Fatalf("ParseTabs: %v", err)
	}
	if len(windows) != 1 || len(windows[0].Tabs) != 2 {
		t.Fatalf("got %+v, want one window with two tabs", windows)
	}
	if windows[0].Tabs[0].URL != "" || windows[0].Tabs[0].Title != "No URL" {
		t.Errorf("first tab = %+v, want empty URL and title %q", windows[0].Tabs[0], "No URL")
	}
}

func TestParseTabsRejoinsTabsInTitles(t *testing.T) {
	windows, err := ParseTabs("1\t1\thttps://go.dev\tA\tB\n")
	if err != nil {
		t.Fatalf("ParseTabs: %v", err)
	}
	if got := windows[0].Tabs[0].Title; got != "A\tB" {
		t.Errorf("title = %q, want %q", got, "A\tB")
	}
}

func TestParseTabsRejectsMalformedLines(t *testing.T) {
	for name, input := range map[string]string{
		"too few fields": "1\t1\tonly-three",
		"bad window":     "x\t1\thttps://go.dev\tGo",
		"bad tab index":  "1\tx\thttps://go.dev\tGo",
	} {
		if _, err := ParseTabs(input); err == nil {
			t.Errorf("%s: ParseTabs(%q) succeeded, want error", name, input)
		}
	}
}

func TestParseTabsEmptyOutput(t *testing.T) {
	windows, err := ParseTabs("\n")
	if err != nil {
		t.Fatalf("ParseTabs: %v", err)
	}
	if len(windows) != 0 {
		t.Errorf("got %+v, want no windows", windows)
	}
}

func TestParseTabsErrorMentionsLine(t *testing.T) {
	_, err := ParseTabs("garbage")
	if err == nil || !strings.Contains(err.Error(), "garbage") {
		t.Errorf("error should mention the offending line, got: %v", err)
	}
}

func TestParseActivation(t *testing.T) {
	cases := []struct {
		output     string
		activation Activation
		found      bool
	}{
		{"ok 3 12", Activation{WindowIndex: 3, TabIndex: 12}, true},
		{"moved 1 5", Activation{WindowIndex: 1, TabIndex: 5, Moved: true}, true},
		{"missing", Activation{}, false},
	}
	for _, c := range cases {
		got, found, err := ParseActivation(c.output)
		if err != nil {
			t.Errorf("ParseActivation(%q): %v", c.output, err)
			continue
		}
		if got != c.activation || found != c.found {
			t.Errorf("ParseActivation(%q) = %+v, %v; want %+v, %v", c.output, got, found, c.activation, c.found)
		}
	}
}

func TestParseActivationRejectsBadOutput(t *testing.T) {
	for _, output := range []string{"", "ok 3", "ok x y", "moved 3 z", "weird 1 2", "ok 1 2 3"} {
		if _, _, err := ParseActivation(output); err == nil {
			t.Errorf("ParseActivation(%q) succeeded, want error", output)
		}
	}
}

func TestEscapeAppleScript(t *testing.T) {
	if got := escapeAppleScript(`a"b\c`); got != `a\"b\\c` {
		t.Errorf("escapeAppleScript = %q, want %q", got, `a\"b\\c`)
	}
}
