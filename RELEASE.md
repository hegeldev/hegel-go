RELEASE_TYPE: minor

- Add `ListGenerator.Unique(bool)` to generate lists whose elements are all distinct. Basic generators propagate uniqueness via the list schema; composite generators reject duplicates through the collection protocol.
- Reject duplicate keys in composite (non-basic) dict generation so `Dicts(...).MinSize(n)` is satisfied even when the key generator produces collisions.
- Omit `max_size` from `new_collection` payloads when unbounded, matching the server's expectation of an absent/None value rather than a negative sentinel.
- Handle StopTest error responses to `collection_reject` by aborting the test case instead of silently ignoring them, which previously caused hangs when too many duplicates were rejected.
