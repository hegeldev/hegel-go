RELEASE_TYPE: patch

Add `WithReplay(blob)` and the repeatable `Workload` flag `--replay` to reproduce failures from a base64 reproduction blob. Only the first blob is replayed, and blobs that no longer fail are reported as failures. Reproduction output now includes a copy-pasteable `hegel.WithReplay("…")` option.
