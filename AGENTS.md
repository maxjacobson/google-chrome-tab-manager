I'd like to write a golang project using charm for gui.

The problem statement is I have too many open tabs open in chrome. and I want a GUI to help me peruse them and mark some to be closed, and then they will be closed.

Here's a quick and dirty fish script that may or may not be a useful reference:

```

#!/usr/bin/env fish

# List the tabs open in Google Chrome, grouped by window.
# Works via AppleScript, so it only runs on macOS.

if test (uname) != Darwin
    echo "list-tabs only works on macOS" >&2
    exit 1
end

if not pgrep -x "Google Chrome" >/dev/null
    echo "Chrome isn't running" >&2
    exit 1
end

set -l applescript '
tell application "Google Chrome"
    set output to ""
    repeat with w from 1 to count of windows
        try
            repeat with t from 1 to count of tabs of window w
                set tabUrl to URL of tab t of window w
                set tabTitle to title of tab t of window w
                if tabUrl is missing value then set tabUrl to ""
                if tabTitle is missing value then set tabTitle to ""
                set output to output & w & "\t" & t & "\t" & tabUrl & "\t" & tabTitle & linefeed
            end repeat
        end try
    end repeat
    return output
end tell
'

set -l tabs (osascript -e $applescript)

for line in $tabs
    set -l fields (string split \t $line)
    set -l window $fields[1]
    set -l tab $fields[2]
    set -l url $fields[3]
    set -l title $fields[4]
    echo "$window.$tab  $title"
    echo "       $url"
end
```

You are the tech lead on this and I (MJ) am the client.

A few asks:

- I'd like you to write a justfile with commands for test, install, and run (running `just` should run it).
- I'd like you to commit and push early and often.
- I'd like you to add a simple github CI and dependabot config.
- I'd like you to use jj for commits, and sign them as you would a git commit with the crush attribution in the description

I'd like you to add a README which discloses that this is vibecoded and other basic info about the project.

Similar projects for reference:

- https://github.com/maxjacobson/seasoning-monitor
- https://github.com/maxjacobson/youtube-rss-feed-fetcher

Please feel free to rewrite this file in your own voice at your earliest convenience.

It's fine if it can only run on macOS.

let's go thru these milestones and pause for my feedback after each one:

1. Let's plan to start with a basic GUI which you can launch and it shows a spinner, and then a list of windows with metadata like tab count. 
2. I can drill down on a window to see the list of tabs in the window and scroll thru the list
3. I can type keywords and filter the tabs by keyword (should search the title and also the URL). Maybe I type `/` to go into the filtering mode and [enter] to exit the filtering mode
4. I can press space to select tabs (or deselect) for deletion and `d` to delete them (with a confirmation dialog)


