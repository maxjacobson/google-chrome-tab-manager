# google-chrome-tab-manager

A small macOS TUI for dealing with a browser that has become a landfill of
open tabs. It asks Google Chrome (via AppleScript) for every open window and
tab, lists the windows, and lets you mark tabs to close and then close them.

**This project is vibecoded**: it was written by an AI coding agent
([Crush](https://charm.land/blog/crush)) with MJ as the client and tech
direction. See `AGENTS.md` for the working notes and milestones.

## Status

All planned milestones are done:

1. ✅ Launch the TUI, see a spinner while it reads Chrome, then a list of
   windows with their tab counts
2. ✅ Drill into a window to see and scroll its tabs
3. ✅ Filter tabs by keyword (`/` to search titles and URLs, enter to exit)
4. ✅ Mark tabs with space and close them with `d` (after a y/n confirmation)
5. ✅ Open a tab in Chrome with `o`
6. ✅ Click a window to drill in, click a tab to focus it

## Usage

```sh
just                # interactive TUI
just run            # same thing
just list-tabs      # print windows and tab counts, then exit
just test           # run the unit tests
just check          # everything CI runs
```

Requires [Go](https://go.dev) and [just](https://github.com/casey/just).
`just install` puts the binary in `$GOBIN`.

### Keys

| Key         | Action                          |
| ----------- | ------------------------------- |
| `enter`     | open the selected window's tabs |
| `esc` / `h` | back to the window list         |
| `j` / `k`   | move selection (or scroll tabs) |
| `g` / `G`   | jump to top / bottom            |
| mouse wheel | scroll the tab list             |
| click       | open a window; focus a tab      |
| `/`         | filter tabs by title and URL    |
| `o`         | show the selected tab in Chrome |
| `space`     | mark (or unmark) a tab to close |
| `d`         | close the marked tabs           |
| `r`         | refresh the tab list            |
| `q`         | quit                            |

## Why macOS only?

Chrome exposes its tabs through AppleScript, which only exists on macOS. The
parse and UI layers are platform-independent, so a Linux backend could be
added later (e.g. via Chrome's DevTools protocol).
