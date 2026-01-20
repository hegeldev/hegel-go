package hegel

// TextGenerator generates text strings with configurable size bounds.
type TextGenerator struct {
	minSize *int
	maxSize *int
}

// Text returns a new text generator.
func Text() *TextGenerator {
	return &TextGenerator{}
}

// MinSize sets the minimum string length (in Unicode codepoints).
func (g *TextGenerator) MinSize(n int) *TextGenerator {
	g.minSize = &n
	return g
}

// MaxSize sets the maximum string length (in Unicode codepoints).
func (g *TextGenerator) MaxSize(n int) *TextGenerator {
	g.maxSize = &n
	return g
}

// Generate produces a text string within the configured bounds.
func (g *TextGenerator) Generate() string {
	return generateFromSchema[string](g.Schema())
}

// Schema returns the JSON schema for this generator.
func (g *TextGenerator) Schema() map[string]any {
	schema := map[string]any{"type": "string"}

	if g.minSize != nil {
		schema["min_length"] = *g.minSize
	}

	if g.maxSize != nil {
		schema["max_length"] = *g.maxSize
	}

	return schema
}

// RegexGenerator generates strings matching a regular expression.
type RegexGenerator struct {
	pattern   string
	fullmatch bool
}

// FromRegex returns a generator for strings that contain a match for the given pattern.
// Use Fullmatch() to require the entire string to match.
func FromRegex(pattern string) *RegexGenerator {
	return &RegexGenerator{pattern: pattern, fullmatch: false}
}

// Fullmatch requires the entire string to match the pattern, not just contain a match.
func (g *RegexGenerator) Fullmatch() *RegexGenerator {
	g.fullmatch = true
	return g
}

// Generate produces a string matching the pattern.
func (g *RegexGenerator) Generate() string {
	return generateFromSchema[string](g.Schema())
}

// Schema returns the JSON schema for this generator.
func (g *RegexGenerator) Schema() map[string]any {
	schema := map[string]any{
		"type":    "regex",
		"pattern": g.pattern,
	}
	if g.fullmatch {
		schema["fullmatch"] = true
	}
	return schema
}
