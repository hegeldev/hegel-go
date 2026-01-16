package hegel

import (
	"testing"
)

func TestDefaultRuns100TestCases(t *testing.T) {
	count := 0

	Hegel(func() {
		Integers[int]().Generate()
		count++
	}, HegelOptions{})

	if count != 100 {
		t.Errorf("expected 100 test cases, got %d", count)
	}
}
