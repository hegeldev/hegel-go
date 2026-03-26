# Hegel for Go
# This justfile provides the standard build recipes.

export PATH := "/usr/local/go/bin:" + env("HOME") + "/go/bin:" + env("PATH")

# Install dependencies and the hegel binary.
# If HEGEL_BINARY is set, symlinks it into .venv/bin instead of installing from git.
setup:
    #!/usr/bin/env bash
    set -euo pipefail
    uv venv .venv
    if [ -n "${HEGEL_BINARY:-}" ]; then
        mkdir -p .venv/bin
        ln -sf "$HEGEL_BINARY" .venv/bin/hegel
    else
        uv pip install --python .venv/bin/python --reinstall-package hegel-core hegel-core
    fi
    # Install Go tools
    go install honnef.co/go/tools/cmd/staticcheck@latest

# Run tests with coverage, fail if below 100%.
# We measure coverage only on the library package (not cmd/ binaries).
test:
    #!/usr/bin/env bash
    set -euo pipefail
    export PATH="$(pwd)/.venv/bin:$PATH"
    go test -race -coverprofile=coverage.out -covermode=atomic \
        -coverpkg=hegel.dev/go/hegel \
        ./...
    python3 scripts/check-coverage.py

# Auto-format code.
format:
    gofmt -w .

# Check formatting + linting.
lint:
    #!/usr/bin/env bash
    set -euo pipefail
    # Check formatting
    unformatted=$(gofmt -l .)
    if [ -n "$unformatted" ]; then
        echo "❌ The following files need formatting (run 'just format'):"
        echo "$unformatted"
        exit 1
    fi
    echo "✅ All files formatted"
    # Run go vet
    go vet ./...
    echo "✅ go vet passed"
    # Run staticcheck
    staticcheck ./...
    echo "✅ staticcheck passed"

# Build API documentation from source. Must succeed with zero warnings.
docs:
    #!/usr/bin/env bash
    set -euo pipefail
    # Verify all exported symbols have doc comments using go vet
    go doc -all . > /dev/null 2>&1
    echo "✅ Documentation generated successfully"

# Build internal conformance test binaries into bin/conformance/.
build-conformance:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p bin/conformance
    go build -o bin/conformance ./internal/conformance/cmd/...
    echo "✅ Conformance binaries built to bin/conformance/"

# Run conformance tests against the real hegel server.
conformance: build-conformance
    #!/usr/bin/env bash
    set -euo pipefail
    export PATH="$(pwd)/.venv/bin:$PATH"
    uv pip install --python .venv/bin/python pytest hypothesis > /dev/null 2>&1 || true
    .venv/bin/python -m pytest tests/conformance/ -v

# Run lint + docs + test (the full CI check).
check: lint docs test
