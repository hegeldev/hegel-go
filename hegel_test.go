package hegel

import "testing"

func TestVersion(t *testing.T) {
	v := Version()
	if v != "0.1.0" {
		t.Errorf("Version() = %q, want %q", v, "0.1.0")
	}
}
