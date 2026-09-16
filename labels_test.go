package hegel

import (
	"fmt"
	"hash/maphash"
	"testing"
)

type labelRecordingTestCase struct {
	TestCase
	labels []label
	depth  int
}

func (tc *labelRecordingTestCase) startSpan(l label) error {
	tc.labels = append(tc.labels, l)
	tc.depth++
	return nil
}

func (tc *labelRecordingTestCase) stopSpan(bool) error {
	tc.depth--
	return nil
}

func (tc *labelRecordingTestCase) inSpan() bool { return tc.depth > 0 }

func (tc *labelRecordingTestCase) reportDraw(int, any) {}

type labelProbeGenerator struct {
	value int
}

func (g labelProbeGenerator) draw(tc TestCase) (int, error) {
	if !tc.inSpan() {
		return 0, fmt.Errorf("generator draw is not inside a span")
	}
	return g.value, nil
}

func TestDrawLabelsGeneratorSpan(t *testing.T) {
	t.Parallel()
	tc := &labelRecordingTestCase{}

	if got := Draw[int](tc, labelProbeGenerator{value: 1}); got != 1 {
		t.Fatalf("first draw = %d, want 1", got)
	}
	Draw[int](tc, labelProbeGenerator{value: 1})
	Draw[int](tc, labelProbeGenerator{value: 2})

	if tc.depth != 0 {
		t.Fatalf("span depth = %d, want 0", tc.depth)
	}
	if len(tc.labels) != 3 {
		t.Fatalf("span labels = %v, want three", tc.labels)
	}
	if tc.labels[0] != tc.labels[1] {
		t.Fatalf("equal generators have labels %d and %d", tc.labels[0], tc.labels[1])
	}
	if tc.labels[0] == tc.labels[2] {
		t.Fatalf("different generators share label %d", tc.labels[0])
	}
}

func TestHashValueFallsBackToType(t *testing.T) {
	t.Parallel()
	seed := maphash.MakeSeed()

	if a, b := hashValue(seed, []int{1}), hashValue(seed, []int{2}); a != b {
		t.Fatalf("non-comparable values of the same type have labels %d and %d", a, b)
	}

	// An interface value can make a statically comparable struct unhashable.
	type runtimeUnhashable struct{ value any }
	if a, b := hashValue(seed, runtimeUnhashable{[]int{1}}), hashValue(seed, runtimeUnhashable{[]int{2}}); a != b {
		t.Fatalf("runtime-unhashable values have labels %d and %d", a, b)
	}
}
