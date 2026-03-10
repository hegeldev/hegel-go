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
	g := Domains()
	bg, ok := g.(*basicGenerator[string])
	if !ok {
		t.Fatalf("Domains() should return *basicGenerator[string], got %T", g)
	}
	if bg.schema["type"] != "domain" {
		t.Errorf("type: expected domain, got %v", bg.schema["type"])
	}
	maxLen, hasMax := bg.schema["max_length"]
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
	g := Domains(DomainMaxLength(63))
	bg, ok := g.(*basicGenerator[string])
	if !ok {
		t.Fatalf("Domains() should return *basicGenerator[string], got %T", g)
	}
	if bg.schema["type"] != "domain" {
		t.Errorf("type: expected domain, got %v", bg.schema["type"])
	}
	maxLen, ok := bg.schema["max_length"]
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

// TestTimesSchema verifies that Times() produces the correct schema.
func TestTimesSchema(t *testing.T) {
	g := Times()
	bg, ok := g.(*basicGenerator[string])
	if !ok {
		t.Fatalf("Times() should return *basicGenerator[string], got %T", g)
	}
	if bg.schema["type"] != "time" {
		t.Errorf("type: expected time, got %v", bg.schema["type"])
	}
	if len(bg.schema) != 1 {
		t.Errorf("Times schema should have exactly 1 key, got %d: %v", len(bg.schema), bg.schema)
	}
}

// TestDatetimesSchema verifies that Datetimes() produces the correct schema.
func TestDatetimesSchema(t *testing.T) {
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
// Unit tests: verify generate sends correct schema over the wire
// =============================================================================

// testGeneratorSchema runs a single generate request using the given generator
// and returns the schema that was received by the fake server.
func testGeneratorSchema[T any](t *testing.T, g Generator[T]) map[any]any {
	t.Helper()
	var gotSchema map[any]any
	clientConn := fakeTestEnv(t, func(caseCh *channel) {
		genID, genPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(genPayload)
		m, _ := extractCBORDict(decoded)
		schemaVal := m[any("schema")]
		gotSchema, _ = extractCBORDict(schemaVal)
		caseCh.SendReplyValue(genID, "test-value") //nolint:errcheck
	})
	cli := newClient(clientConn)
	cli.runTest("schema_check", func(s *TestCase) { //nolint:errcheck
		g.draw(s)
	}, runOptions{testCases: 1}, stderrNoteFn)
	return gotSchema
}

// TestEmailsGeneratesCorrectSchema checks the wire schema for Emails().
func TestEmailsGeneratesCorrectSchema(t *testing.T) {
	schema := testGeneratorSchema(t, Emails())
	if schema[any("type")] != "email" {
		t.Errorf("generate schema type: expected email, got %v", schema[any("type")])
	}
}

// TestURLsGeneratesCorrectSchema checks the wire schema for URLs().
func TestURLsGeneratesCorrectSchema(t *testing.T) {
	schema := testGeneratorSchema(t, URLs())
	if schema[any("type")] != "url" {
		t.Errorf("generate schema type: expected url, got %v", schema[any("type")])
	}
}

// TestDatesGeneratesCorrectSchema checks the wire schema for Dates().
func TestDatesGeneratesCorrectSchema(t *testing.T) {
	schema := testGeneratorSchema(t, Dates())
	if schema[any("type")] != "date" {
		t.Errorf("generate schema type: expected date, got %v", schema[any("type")])
	}
}

// TestTimesGeneratesCorrectSchema checks the wire schema for Times().
func TestTimesGeneratesCorrectSchema(t *testing.T) {
	schema := testGeneratorSchema(t, Times())
	if schema[any("type")] != "time" {
		t.Errorf("generate schema type: expected time, got %v", schema[any("type")])
	}
}

// TestDatetimesGeneratesCorrectSchema checks the wire schema for Datetimes().
func TestDatetimesGeneratesCorrectSchema(t *testing.T) {
	schema := testGeneratorSchema(t, Datetimes())
	if schema[any("type")] != "datetime" {
		t.Errorf("generate schema type: expected datetime, got %v", schema[any("type")])
	}
}

// TestDomainsGeneratesCorrectSchemaNoMax checks the wire schema for Domains() with default max_length.
func TestDomainsGeneratesCorrectSchemaNoMax(t *testing.T) {
	schema := testGeneratorSchema(t, Domains())
	if schema[any("type")] != "domain" {
		t.Errorf("generate schema type: expected domain, got %v", schema[any("type")])
	}
	maxLen, hasMax := schema[any("max_length")]
	if !hasMax {
		t.Fatal("max_length should always be in domain schema")
	}
	ml, _ := extractCBORInt(maxLen)
	if ml != 255 {
		t.Errorf("default max_length: expected 255, got %d", ml)
	}
}

// TestDomainsGeneratesCorrectSchemaWithMax checks the wire schema for Domains() with max_length.
func TestDomainsGeneratesCorrectSchemaWithMax(t *testing.T) {
	schema := testGeneratorSchema(t, Domains(DomainMaxLength(30)))
	if schema[any("type")] != "domain" {
		t.Errorf("generate schema type: expected domain, got %v", schema[any("type")])
	}
	maxLen, ok := schema[any("max_length")]
	if !ok {
		t.Fatal("max_length should be in schema when MaxLength > 0")
	}
	ml, _ := extractCBORInt(maxLen)
	if ml != 30 {
		t.Errorf("max_length: expected 30, got %d", ml)
	}
}

// =============================================================================
// E2E integration tests: property tests with the real hegel binary
// =============================================================================

// TestEmailsE2E verifies that generated emails contain "@".
func TestEmailsE2E(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := Draw(s, Emails())
		if !strings.Contains(v, "@") {
			panic("email does not contain '@': " + v)
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}

// TestURLsE2E verifies that generated URLs start with "http://" or "https://".
func TestURLsE2E(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := Draw(s, URLs())
		if !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://") {
			panic("url does not start with http:// or https://: " + v)
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}

// isValidDomainChar returns true if r is a valid character in a domain label.
func isValidDomainChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '.'
}

// TestDomainsE2E verifies that generated domains contain only valid domain characters.
func TestDomainsE2E(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := Draw(s, Domains())
		for _, r := range v {
			if !isValidDomainChar(r) {
				panic("domain contains invalid character '" + string(r) + "': " + v)
			}
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}

// TestDomainsMaxLengthE2E verifies that generated domains respect the max_length constraint.
func TestDomainsMaxLengthE2E(t *testing.T) {
	hegelBinPath(t)
	const maxLen = 20
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := Draw(s, Domains(DomainMaxLength(maxLen)))
		if len(v) > maxLen {
			panic("domain exceeds max_length constraint: " + v)
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}

// TestDatesE2E verifies that generated dates are valid time.Time values.
func TestDatesE2E(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := Draw(s, Dates())
		if v.IsZero() {
			panic("date is zero value")
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}

// TestTimesE2E verifies that generated times contain ":".
func TestTimesE2E(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := Draw(s, Times())
		if !strings.Contains(v, ":") {
			panic("time does not contain ':': " + v)
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}

// TestDatetimesE2E verifies that generated datetimes are valid time.Time values.
func TestDatetimesE2E(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := Draw(s, Datetimes())
		if v.IsZero() {
			panic("datetime is zero value")
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}
