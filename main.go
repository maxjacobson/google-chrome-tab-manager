// Command google-chrome-tab-manager helps close a pile of open Chrome tabs.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/maxjacobson/google-chrome-tab-manager/internal/chrome"
	"github.com/maxjacobson/google-chrome-tab-manager/internal/ui"
)

func main() {
	list := flag.Bool("list", false, "print the open windows and tab counts, then exit")
	flag.Parse()

	if err := run(*list); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(list bool) error {
	if list {
		windows, err := chrome.Chrome{}.ListTabs(context.Background())
		if err != nil {
			return err
		}
		for _, w := range windows {
			fmt.Printf("window %d  %d tabs\n", w.Index, len(w.Tabs))
		}
		return nil
	}
	p := tea.NewProgram(ui.New(context.Background(), chrome.Chrome{}))
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("running the UI: %w", err)
	}
	return nil
}
