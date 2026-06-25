RELEASE_TYPE: patch

This release hands rule selection in stateful tests (`RunStateful`) to the
libhegel engine. Previously hegel-go drew the next rule itself; it now asks the
engine via the state-machine protocol, which applies swarm testing: each test
case enables a random subset of the rules and draws only from that subset, with
the restrictions shrinking away in minimal counterexamples. This tends to
surface bugs that only appear under particular combinations of rules.
