RELEASE_TYPE: patch

This release vendors the supported platform builds of `libhegel` and embeds them in the Go module. Hegel no longer downloads `libhegel` from GitHub at runtime, so builds are self-contained and work offline. `HEGEL_LIBHEGEL_PATH` remains available for loading a custom library.
