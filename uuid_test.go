//go:build go1.27

package hegel

import (
	"hegel.dev/go/hegel/internal/libhegel"
	"testing"
	"uuid"
)

func TestUUIDs(t *testing.T) {
	gen := UUIDs()

	err := run(1, func(tc TestCase) {
		got := Draw(tc, gen)
		if got == (uuid.UUID{}) {
			tc.Errorf("UUIDs generated the nil UUID")
		}
		if _, err := uuid.Parse(got.String()); err != nil {
			tc.Errorf("UUIDs generated an invalid UUID %q: %v", got, err)
		}
	}, WithTestCases(1))
	if err != nil {
		t.Fatalf("run UUIDs generator: %v", err)
	}
}

func TestUUIDsVersion(t *testing.T) {
	err := run(1, func(tc TestCase) {
		got := Draw(tc, UUIDs().Version(4))
		if version := got[6] >> 4; version != 4 {
			tc.Errorf("UUIDs().Version(4) generated version %d UUID %q", version, got)
		}
		if variant := got[8] >> 4; variant < 8 || variant > 11 {
			tc.Errorf("UUIDs().Version(4) generated non-RFC 4122 variant UUID %q", got)
		}
	}, WithTestCases(1))
	if err != nil {
		t.Fatalf("run versioned UUID generator: %v", err)
	}
}

func TestUUIDsInvalidVersion(t *testing.T) {
	_, err := UUIDs().Version(6).draw(nil)
	assertErrorContains(t, "Version must be between 1 and 5", err)
}

func TestDefaultUUIDUsesUUIDs(t *testing.T) {
	Test(t, func(tc *T) {
		got := Draw(tc, Default[struct{ UUID uuid.UUID }]()).UUID
		if got == (uuid.UUID{}) {
			tc.Fatal("Default generated the nil UUID")
		}
		if _, err := uuid.Parse(got.String()); err != nil {
			tc.Fatalf("invalid UUID: %v", err)
		}
	}, WithTestCases(10))
}

func TestDefaultUUIDDelegates(t *testing.T) {
	want := uuid.UUID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	got, err := Default[uuid.UUID]().draw(newStubTestCase(t, libhegel.OK, want[:], libhegel.OK, libhegel.OK))
	if err != nil || got != want {
		t.Fatalf("got %v, %v; want %v", got, err, want)
	}
}
