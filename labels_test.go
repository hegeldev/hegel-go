package hegel

import (
	"fmt"
	"slices"
	"testing"

	"hegel.dev/go/hegel/internal/libhegel"
)

type labelRecordingTestCase struct {
	TestCase
	labels []libhegel.Label
	depth  int
}

func (tc *labelRecordingTestCase) startSpan(l libhegel.Label) error {
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

type otherLabelProbeGenerator struct{}

func (otherLabelProbeGenerator) draw(TestCase) (int, error) { return 0, nil }

func TestDrawLabelsGeneratorSpan(t *testing.T) {
	t.Parallel()
	tc := &labelRecordingTestCase{}

	if got := Draw[int](tc, labelProbeGenerator{value: 1}); got != 1 {
		t.Fatalf("first draw = %d, want 1", got)
	}
	Draw[int](tc, labelProbeGenerator{value: 2})
	Draw[int](tc, otherLabelProbeGenerator{})

	if tc.depth != 0 {
		t.Fatalf("span depth = %d, want 0", tc.depth)
	}
	if len(tc.labels) != 3 {
		t.Fatalf("span labels = %v, want three", tc.labels)
	}
	if tc.labels[0] != tc.labels[1] {
		t.Fatalf("same generator type has labels %d and %d", tc.labels[0], tc.labels[1])
	}
	if tc.labels[0] == tc.labels[2] {
		t.Fatalf("different generator types share label %d", tc.labels[0])
	}
}

func TestDrawLabelsInnerGenerator(t *testing.T) {
	t.Parallel()
	tc := &labelRecordingTestCase{}
	inner := labelProbeGenerator{value: 1}
	outer := Map(inner, func(value int) int { return value + 1 })

	if got := Draw(tc, outer); got != 2 {
		t.Fatalf("draw = %d, want 2", got)
	}
	want := []libhegel.Label{labelFor(outer), labelFor(inner)}
	if !slices.Equal(tc.labels, want) {
		t.Fatalf("span labels = %v, want %v", tc.labels, want)
	}
}
