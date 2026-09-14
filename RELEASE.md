RELEASE_TYPE: patch

Fix linking tests with `CGO_ENABLED=0` on macOS ARM64, which previously failed with a duplicate `_cgo_init` symbol.
