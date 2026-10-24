# Common tasks for google-chrome-tab-manager.

# Run the interactive TUI (the default: just type `just`).
default: run

# List available recipes.
list:
    @just --list

# Build the binary into ./google-chrome-tab-manager.
build:
    go build -o google-chrome-tab-manager .

# Install the binary into $GOBIN (usually ~/go/bin) so it runs from anywhere.
install:
    go install .

# Run the interactive TUI.
run:
    go run .

# Print the open windows and their tab counts, then exit.
list-tabs:
    go run . --list

# Run the unit tests.
test:
    go test ./...

# Run go vet.
vet:
    go vet ./...

# Report the warnings gopls (Zed's Go language server) would show.
lint:
    #!/usr/bin/env sh
    if ! command -v gopls >/dev/null 2>&1; then
        echo "gopls not found; install it with: go install golang.org/x/tools/gopls@latest" >&2
        exit 1
    fi
    out=$(gopls check $(fd -e go))
    if [ -n "$out" ]; then
        printf '%s\n' "$out" >&2
        exit 1
    fi

# Format all Go files.
fmt:
    gofmt -w .

# Fail if any file needs formatting.
fmt-check:
    test -z "$(gofmt -l .)"

# Everything CI would run: formatting, vet, editor warnings, and tests.
check: fmt-check vet lint test

# Remove the built binary.
clean:
    rm -f google-chrome-tab-manager
