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
#
# The library only uses a local libhegel when HEGEL_LIBHEGEL_PATH points at one;
# otherwise it auto-downloads the pinned, checksum-verified release. With a
# sibling hegel-rust checkout present, this recipe builds it and exports that
# path. Pass `vendored` as the mode to skip the local build and test against the
# pinned release instead — handy after a version bump, when the checkout may lag
# hegelVersion. An externally set HEGEL_LIBHEGEL_PATH (e.g. in CI) always wins.
test mode="" *args="":
    #!/usr/bin/env bash
    set -euo pipefail
    case "{{mode}}" in
        vendored)
            unset HEGEL_LIBHEGEL_PATH ;;
        "")
            just build-libhegel
            if [ -z "${HEGEL_LIBHEGEL_PATH:-}" ]; then
                ext=so; [ "$(uname)" = Darwin ] && ext=dylib
                libpath="{{hegel_rust_dir}}/target/release/libhegel.$ext"
                [ -f "$libpath" ] && export HEGEL_LIBHEGEL_PATH="$libpath"
            fi ;;
        *)
            echo "unknown test mode '{{mode}}' (expected '' or 'vendored')" >&2
            exit 1 ;;
    esac
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

# Run lint + docs + test (the full CI check). Pass `vendored` to test against
# the pinned libhegel release instead of a local hegel-rust checkout.
check mode="": lint check-docs (test mode)
