export PATH := "/usr/local/go/bin:" + env("HOME") + "/go/bin:" + env("PATH")

# Run tests with coverage.
# We measure coverage only on the library package (not cmd/ binaries).
test:
    go test -race -coverprofile=coverage.out -covermode=atomic \
        -coverpkg=hegel.dev/go/hegel \
        ./...
    python3 scripts/check-coverage.py

format:
    gofmt -w .

lint:
    #!/usr/bin/env bash
    set -euo pipefail
    unformatted=$(gofmt -l .)
    if [ -n "$unformatted" ]; then
        echo "The following files need formatting (run 'just format'):"
        echo "$unformatted"
        exit 1
    fi
    go vet ./...
    go tool staticcheck ./...

check-docs:
    # verify all exported symbols have doc comments
    go doc -all . > /dev/null 2>&1

docs:
    go doc -http .

build-conformance:
    mkdir -p bin/conformance
    go build -o bin/conformance ./internal/conformance/cmd/...

conformance: build-conformance
    uv run --with 'hegel-core==0.5.0' --with pytest --with hypothesis \
        pytest tests/conformance/ -v

# Run lint + docs + test (the full CI check).
check: lint check-docs test
