RELEASE_TYPE: patch

`Default()` now uses specialized generators for `time.Time`, `netip.Addr`, and (on Go 1.27 and later) `uuid.UUID`, including inside composite types.
