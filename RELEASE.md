RELEASE_TYPE: patch

This patch fixes a rare crash. A wrapper's GC cleanup could free its
native libhegel handle while a call on that same handle was still
executing, because the raw handle is invisible to the collector and the
wrapper's last use can precede the call's return (observed once as a
segfault inside `hegel_mark_complete`). Every native call now pins its
handle's wrapper with `runtime.KeepAlive`, and a test enforces the rule
for all future bindings.
