RELEASE_TYPE: patch

`Default()` now caches one generator per type, eliminating allocations on repeated calls.
