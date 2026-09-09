package hegel

import (
	"io"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestTimezoneNamesWellFormed guards the generated candidate list: UTC first
// (the shrink target), the rest sorted and unique, Factory absent, and every
// name resolvable through the embedded tz database.
func TestTimezoneNamesWellFormed(t *testing.T) {
	t.Parallel()
	if len(timezoneNames) == 0 || timezoneNames[0] != "UTC" {
		t.Fatal("timezoneNames must start with UTC")
	}
	rest := timezoneNames[1:]
	if !slices.IsSorted(rest) {
		t.Error("timezoneNames[1:] is not sorted")
	}
	if slices.Contains(rest, "UTC") {
		t.Error("timezoneNames lists UTC more than once")
	}
	if slices.Contains(rest, "Factory") {
		t.Error("timezoneNames must not list the Factory placeholder zone")
	}
	if compact := slices.Compact(slices.Clone(rest)); len(compact) != len(rest) {
		t.Errorf("timezoneNames has %d duplicate names", len(rest)-len(compact))
	}
	for _, name := range timezoneNames {
		if _, err := time.LoadLocation(name); err != nil {
			t.Errorf("LoadLocation(%q): %v", name, err)
		}
	}
}

// TestLoadTimezoneCachesLocations verifies repeated lookups return the cached
// location and that UTC resolves to time.UTC itself, which is what makes
// shrunk examples compare equal to time.UTC.
func TestLoadTimezoneCachesLocations(t *testing.T) {
	t.Parallel()
	first, err := loadTimezone("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadTimezone("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Error("expected the cached *time.Location on the second lookup")
	}
	utc, err := loadTimezone("UTC")
	if err != nil {
		t.Fatal(err)
	}
	if utc != time.UTC {
		t.Errorf("loadTimezone(UTC) = %v, want time.UTC", utc)
	}
}

// TestLoadTimezoneUnknownName covers the LoadLocation failure path; the error
// names the offending zone.
func TestLoadTimezoneUnknownName(t *testing.T) {
	t.Parallel()
	_, err := loadTimezone("Not/A_Zone")
	if err == nil || !strings.Contains(err.Error(), "Not/A_Zone") {
		t.Fatalf("expected an error naming the zone, got %v", err)
	}
}

// TestTimezonesShrinkToUTC: a property that fails for every zone shrinks to
// the simplest one, UTC, which sits first in the candidate list. The final
// replay of the minimal example is the last time the body runs.
func TestTimezonesShrinkToUTC(t *testing.T) {
	t.Parallel()
	var last *time.Location
	err := run(func(tc TestCase) {
		last = Draw(tc, Timezones())
		tc.Fail()
	}, WithTestCases(20), WithDatabase(""), withOutput(io.Discard))
	if err == nil {
		t.Fatal("expected the property to fail")
	}
	if last != time.UTC {
		t.Errorf("minimal failing timezone = %v, want UTC", last)
	}
}
