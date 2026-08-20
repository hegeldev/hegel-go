//go:build go1.27

package hegel

import (
	"testing"
	"uuid"
)

func TestUUIDs(t *testing.T) {
	gen := UUIDs()

	err := run(func(tc TestCase) {
		got := Draw(tc, gen)
		if got == (uuid.UUID{}) {
			tc.Errorf("UUIDs generated the nil UUID")
		}
		if _, err := uuid.Parse(got.String()); err != nil {
			tc.Errorf("UUIDs generated an invalid UUID %q: %v", got, err)
		}
	}, WithSingleTestCase())
	if err != nil {
		t.Fatalf("run UUIDs generator: %v", err)
	}
}

func TestUUIDsVersion(t *testing.T) {
	err := run(func(tc TestCase) {
		got := Draw(tc, UUIDs().Version(4))
		if version := got[6] >> 4; version != 4 {
			tc.Errorf("UUIDs().Version(4) generated version %d UUID %q", version, got)
		}
		if variant := got[8] >> 4; variant < 8 || variant > 11 {
			tc.Errorf("UUIDs().Version(4) generated non-RFC 4122 variant UUID %q", got)
		}
	}, WithSingleTestCase())
	if err != nil {
		t.Fatalf("run versioned UUID generator: %v", err)
	}
}

func TestUUIDsInvalidVersion(t *testing.T) {
	_, err := UUIDs().Version(6).draw(nil)
	assertErrorContains(t, "Version must be between 1 and 5", err)
}
