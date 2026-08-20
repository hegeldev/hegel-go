//go:build go1.27

package hegel

import (
	"fmt"
	"uuid"
)

// UUIDGenerator configures and generates UUID values.
// Use [UUIDs] to create one, then chain builder methods to configure it.
type UUIDGenerator struct {
	version    int
	hasVersion bool
}

var _ Generator[uuid.UUID] = UUIDGenerator{}

// UUIDs returns a Generator that produces UUID values.
func UUIDs() UUIDGenerator {
	return UUIDGenerator{}
}

// Version restricts the generator to a specific RFC 4122 version (1–5).
func (g UUIDGenerator) Version(version int) UUIDGenerator {
	g.version = version
	g.hasVersion = true
	return g
}

func (g UUIDGenerator) draw(tc TestCase) (uuid.UUID, error) {
	if g.hasVersion && (g.version < 1 || g.version > 5) {
		return uuid.UUID{}, fmt.Errorf("UUIDs: Version must be between 1 and 5, got %d", g.version)
	}
	ctx, ltc := tc.engine()
	b, err := ltc.GenerateUUID(ctx, uint8(g.version), g.hasVersion)
	if err != nil {
		return uuid.UUID{}, err
	}
	return uuid.UUID(b), nil
}
