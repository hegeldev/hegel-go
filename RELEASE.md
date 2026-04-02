RELEASE_TYPE: patch

Drop the `golang.org/x/exp/constraints` dependency in favor of locally defined `Integer` and `Float` type constraints. These interfaces are identical to the ones from `x/exp` but eliminate an external dependency now that Go's type parameter support is mature.
