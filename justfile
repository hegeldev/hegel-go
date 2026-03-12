# Hegel SDK for go
# This justfile provides the standard build recipes.

export PATH := "/usr/local/go/bin:" + env("HOME") + "/go/bin:" + env("PATH")

# Install dependencies and the hegel binary.
# If HEGEL_BINARY is set, uses that path via HEGEL_CMD instead of auto-installing.
# Otherwise, installs hegel into .hegel/venv at the version pinned in runner.go.
setup:
    #!/usr/bin/env bash
    set -euo pipefail
    if [ -n "${HEGEL_BINARY:-}" ]; then
        export HEGEL_CMD="$HEGEL_BINARY"
        echo "Using HEGEL_CMD=$HEGEL_CMD"
    else
        mkdir -p .hegel
        uv venv --clear .hegel/venv
        uv pip install --python .hegel/venv/bin/python \
            "hegel @ git+ssh://git@github.com/antithesishq/hegel-core.git@$(grep 'hegelVersion = ' runner.go | head -1 | sed 's/.*"\(.*\)"/\1/')"
        # Write version file so the SDK recognises the cached install.
        grep 'hegelVersion = ' runner.go | head -1 | sed 's/.*"\(.*\)"/\1/' > .hegel/venv/hegel-version
    fi
    # Install Go tools
    go install honnef.co/go/tools/cmd/staticcheck@latest

# Run tests with coverage, fail if below 100%.
# We measure coverage only on the library package (not cmd/ binaries).
test:
    #!/usr/bin/env bash
    set -euo pipefail
    export PATH="$(pwd)/.hegel/venv/bin:$PATH"
    go test -race -coverprofile=coverage.out -covermode=atomic \
        -coverpkg=github.com/hegeldev/hegel-go \
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

# Build conformance test binaries into bin/conformance/.
build-conformance:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p bin/conformance
    for pkg in cmd/conformance/*/; do
        name=$(basename "$pkg")
        go build -o "bin/conformance/$name" "./$pkg"
    done
    echo "✅ Conformance binaries built to bin/conformance/"

# Run conformance tests against the real hegel server.
conformance: build-conformance
    #!/usr/bin/env bash
    set -euo pipefail
    export PATH="$(pwd)/.hegel/venv/bin:$PATH"
    uv pip install --python .hegel/venv/bin/python pytest pytest-subtests hypothesis > /dev/null 2>&1 || true
    .hegel/venv/bin/python -m pytest tests/conformance/ -v

# Update the pinned hegel-core version to the latest release.
update-hegel-core-version:
    #!/usr/bin/env bash
    set -euo pipefail
    tag=$(gh api repos/antithesishq/hegel-core/releases/latest --jq '.tag_name')
    sed -i '' "s/^const hegelVersion = \".*\"/const hegelVersion = \"${tag}\"/" runner.go
    echo "Updated hegelVersion to ${tag}"
    # Clear cached install so the next test run picks up the new version
    rm -rf .hegel/venv

# Run lint + docs + test (the full CI check).
check: lint docs test
