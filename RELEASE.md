RELEASE_TYPE: patch

Add `WithReplay(blob)` and the repeatable `Workload` flag `--replay` to reproduce failures from a base64 reproduction blob. The first non-empty `WithReplay` option and first `--replay` flag take precedence. Blobs that no longer fail are reported as failures. Reproduction output now includes a copy-pasteable `hegel.WithReplay("…")` option.
