RELEASE_TYPE: patch

Stateful invariants are now checked unconditionally on the initial and final
states and sampled at intermediate join points. Use
`WithAlwaysCheckInvariants` for invariants that must observe every intermediate
state.
