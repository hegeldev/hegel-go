package hegel

// formats_test.go tests email, url, domain, date, time, and datetime generators.

import (
	"strings"
	"testing"
	"time"
	"unicode"
)

// =============================================================================
// Unit tests: verify schema structure for format generators
// =============================================================================

// TestEmailsSchema verifies that Emails() produces the correct schema.
func TestEmailsSchema(t *testing.T) {
	t.Parallel()
	g := Emails()
	bg, ok := g.(*basicGenerator[string])
	if !ok {
		t.Fatalf("Emails() should return *basicGenerator[string], got %T", g)
	}
	if bg.schema["type"] != "email" {
		t.Errorf("type: expected email, got %v", bg.schema["type"])
	}
	if len(bg.schema) != 1 {
		t.Errorf("Emails schema should have exactly 1 key, got %d: %v", len(bg.schema), bg.schema)
	}
}

// TestURLsSchema verifies that URLs() produces the correct schema.
func TestURLsSchema(t *testing.T) {
	t.Parallel()
	g := URLs()
	bg, ok := g.(*basicGenerator[string])
	if !ok {
		t.Fatalf("URLs() should return *basicGenerator[string], got %T", g)
	}
	if bg.schema["type"] != "url" {
		t.Errorf("type: expected url, got %v", bg.schema["type"])
	}
	if len(bg.schema) != 1 {
		t.Errorf("URLs schema should have exactly 1 key, got %d: %v", len(bg.schema), bg.schema)
	}
}

// TestDomainsSchemaNoMaxLength verifies that Domains() with no MaxLength uses the default (255).
func TestDomainsSchemaNoMaxLength(t *testing.T) {
	t.Parallel()
	g := Domains()
	schema := g.buildSchema()
	if schema["type"] != "domain" {
		t.Errorf("type: expected domain, got %v", schema["type"])
	}
	maxLen, hasMax := schema["max_length"]
	if !hasMax {
		t.Fatal("max_length should always be present in domain schema")
	}
	ml, _ := extractCBORInt(maxLen)
	if ml != 255 {
		t.Errorf("default max_length: expected 255, got %d", ml)
	}
}

// TestDomainsSchemaWithMaxLength verifies that Domains() with MaxLength includes it.
func TestDomainsSchemaWithMaxLength(t *testing.T) {
	t.Parallel()
	g := Domains().MaxLength(63)
	schema := g.buildSchema()
	if schema["type"] != "domain" {
		t.Errorf("type: expected domain, got %v", schema["type"])
	}
	maxLen, ok := schema["max_length"]
	if !ok {
		t.Fatal("max_length should be present when MaxLength > 0")
	}
	ml, _ := extractCBORInt(maxLen)
	if ml != 63 {
		t.Errorf("max_length: expected 63, got %d", ml)
	}
}

// TestDatesSchema verifies that Dates() produces the correct schema.
func TestDatesSchema(t *testing.T) {
	t.Parallel()
	g := Dates()
	bg, ok := g.(*basicGenerator[time.Time])
	if !ok {
		t.Fatalf("Dates() should return *basicGenerator[time.Time], got %T", g)
	}
	if bg.schema["type"] != "date" {
		t.Errorf("type: expected date, got %v", bg.schema["type"])
	}
	if len(bg.schema) != 1 {
		t.Errorf("Dates schema should have exactly 1 key, got %d: %v", len(bg.schema), bg.schema)
	}
}

// TestDatetimesSchema verifies that Datetimes() produces the correct schema.
func TestDatetimesSchema(t *testing.T) {
	t.Parallel()
	g := Datetimes()
	bg, ok := g.(*basicGenerator[time.Time])
	if !ok {
		t.Fatalf("Datetimes() should return *basicGenerator[time.Time], got %T", g)
	}
	if bg.schema["type"] != "datetime" {
		t.Errorf("type: expected datetime, got %v", bg.schema["type"])
	}
	if len(bg.schema) != 1 {
		t.Errorf("Datetimes schema should have exactly 1 key, got %d: %v", len(bg.schema), bg.schema)
	}
}

// =============================================================================
// E2E integration tests: property tests with the real hegel binary
// =============================================================================

// TestEmailsE2E verifies that generated emails contain "@".
func TestEmailsE2E(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	if _err := Run(func(s *TestCase) {
		v := Draw(s, Emails())
		if !strings.Contains(v, "@") {
			panic("email does not contain '@': " + v)
		}
	}, WithTestCases(30)); _err != nil {
		panic(_err)
	}
}

// TestURLsE2E verifies that generated URLs start with "http://" or "https://".
func TestURLsE2E(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	if _err := Run(func(s *TestCase) {
		v := Draw(s, URLs())
		if !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://") {
			panic("url does not start with http:// or https://: " + v)
		}
	}, WithTestCases(30)); _err != nil {
		panic(_err)
	}
}

// isValidDomainChar returns true if r is a valid character in a domain label.
func isValidDomainChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '.'
}

// TestDomainsE2E verifies that generated domains contain only valid domain characters.
func TestDomainsE2E(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	if _err := Run(func(s *TestCase) {
		v := Draw(s, Domains())
		for _, r := range v {
			if !isValidDomainChar(r) {
				panic("domain contains invalid character '" + string(r) + "': " + v)
			}
		}
	}, WithTestCases(30)); _err != nil {
		panic(_err)
	}
}

// TestDomainsMaxLengthE2E verifies that generated domains respect the max_length constraint.
func TestDomainsMaxLengthE2E(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	const maxLen = 20
	if _err := Run(func(s *TestCase) {
		v := Draw(s, Domains().MaxLength(maxLen))
		if len(v) > maxLen {
			panic("domain exceeds max_length constraint: " + v)
		}
	}, WithTestCases(30)); _err != nil {
		panic(_err)
	}
}

// TestDatesE2E verifies that generated dates are valid time.Time values.
func TestDatesE2E(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	if _err := Run(func(s *TestCase) {
		v := Draw(s, Dates())
		if v.IsZero() {
			panic("date is zero value")
		}
	}, WithTestCases(30)); _err != nil {
		panic(_err)
	}
}

// TestDatetimesE2E verifies that generated datetimes are valid time.Time values.
func TestDatetimesE2E(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	if _err := Run(func(s *TestCase) {
		v := Draw(s, Datetimes())
		if v.IsZero() {
			panic("datetime is zero value")
		}
	}, WithTestCases(30)); _err != nil {
		panic(_err)
	}
}
