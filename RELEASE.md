RELEASE_TYPE: patch

This release fixes two issues in stateful testing:

- Calling `Assume()` inside an invariant now rejects only that invariant
  invocation rather than the entire test case, matching the behaviour of
  `Assume()` inside a rule.
- The number of steps taken during a stateful run is now drawn with a
  skewed distribution clamped to the maximum step count, aligning step
  selection with the other Hegel libraries so that shrinking behaves
  consistently.
