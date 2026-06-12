export PATH := "/usr/local/go/bin:" + env("HOME") + "/go/bin:" + env("PATH")

# Path to the libhegel-rust checkout (override with HEGEL_RUST_DIR=...).
hegel_rust_dir := env("HEGEL_RUST_DIR", justfile_directory() + "/../hegel-rust")

# Build libhegel.{so,dylib} in the sibling hegel-rust checkout. No-op when
# the checkout isn't present — tests fall back to the auto-downloader,
# which fetches the matching version from hegel-rust's GitHub releases.
build-libhegel:
    #!/usr/bin/env bash
    set -euo pipefail
    if [ -d "{{hegel_rust_dir}}/hegel-c" ]; then
        cd "{{hegel_rust_dir}}" && cargo build -p hegeltest-c --release
    else
        echo "No hegel-rust checkout at {{hegel_rust_dir}}; tests will auto-download libhegel from GitHub releases."
    fi

# Run tests with coverage.
# We measure coverage across all packages; exclusions are in .testcoverage.yml.
test *args: build-libhegel
    go test -race {{args}} -coverprofile=coverage.out -covermode=atomic \
        -coverpkg=hegel.dev/go/hegel/... \
        ./...
    python3 scripts/check-coverage.py

format:
    gofmt -w .

# Bump the pinned hegel-rust release: regenerate hegelVersion and the baked-in
# libhegel checksums in checksums.go, then commit the result. With no argument
# this targets the latest release; pass a version (e.g. `just update-checksums
# 0.17.5`) to pin that exact release.
update-checksums version="":
    go run scripts/update-checksums.go -version={{version}} -- internal/libhegel/checksums.go
    gofmt -w internal/libhegel/checksums.go

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

# Run lint + docs + test (the full CI check).
check: lint check-docs test
