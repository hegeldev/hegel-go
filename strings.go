package hegel

import "strings"

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
		schema["minLength"] = *g.minSize
	}

	if g.maxSize != nil {
		schema["maxLength"] = *g.maxSize
	}

	return schema
}

// RegexGenerator generates strings matching a regular expression.
type RegexGenerator struct {
	pattern string
}

// FromRegex returns a generator for strings matching the given pattern.
// The pattern is automatically anchored with ^ and $ if not already present.
func FromRegex(pattern string) *RegexGenerator {
	anchored := pattern
	if !strings.HasPrefix(pattern, "^") {
		anchored = "^" + anchored
	}
	if !strings.HasSuffix(anchored, "$") {
		anchored = anchored + "$"
	}
	return &RegexGenerator{pattern: anchored}
}

// Generate produces a string matching the pattern.
func (g *RegexGenerator) Generate() string {
	return generateFromSchema[string](g.Schema())
}

// Schema returns the JSON schema for this generator.
func (g *RegexGenerator) Schema() map[string]any {
	return map[string]any{
		"type":    "string",
		"pattern": g.pattern,
	}
}
