package hegel

import "encoding/base64"

// BinaryGenerator generates binary data (byte sequences) with configurable size bounds.
type BinaryGenerator struct {
	minSize *int
	maxSize *int
}

// Binary returns a new binary generator.
func Binary() *BinaryGenerator {
	return &BinaryGenerator{}
}

// MinSize sets the minimum size in bytes.
func (g *BinaryGenerator) MinSize(n int) *BinaryGenerator {
	g.minSize = &n
	return g
}

// MaxSize sets the maximum size in bytes.
func (g *BinaryGenerator) MaxSize(n int) *BinaryGenerator {
	g.maxSize = &n
	return g
}

// Generate produces binary data within the configured bounds.
func (g *BinaryGenerator) Generate() []byte {
	b64 := generateFromSchema[string](g.Schema())
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		panic("hegel: invalid base64 from server: " + err.Error())
	}
	return data
}

// Schema returns the JSON schema for this generator.
func (g *BinaryGenerator) Schema() map[string]any {
	minSize := 0
	if g.minSize != nil {
		minSize = *g.minSize
	}

	schema := map[string]any{
		"type":     "binary",
		"min_size": minSize,
	}

	if g.maxSize != nil {
		schema["max_size"] = *g.maxSize
	}

	return schema
}
