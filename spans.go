package hegel

// Span labels help Hypothesis understand the structure of generated data.
// These improve test shrinking by keeping related values together.
const (
	LabelList        uint64 = 1
	LabelListElement uint64 = 2
	LabelSet         uint64 = 3
	LabelSetElement  uint64 = 4
	LabelMap         uint64 = 5
	LabelMapEntry    uint64 = 6
	LabelTuple       uint64 = 7
	LabelOneOf       uint64 = 8
	LabelOptional    uint64 = 9
	LabelFixedDict   uint64 = 10
	LabelFlatMap     uint64 = 11
	LabelFilter      uint64 = 12
	LabelSampledFrom uint64 = 13
)

// StartSpan begins a labeled group of generation calls.
// This opens a connection if not already connected.
// You must call StopSpan() for each StartSpan().
func StartSpan(label uint64) {
	if !isConnected() {
		openConnection()
	}
	incrementSpanDepth()
	sendRequest("start_span", map[string]any{"label": label})
}

// StopSpan ends the current generation span.
// If discard is true, tells Hypothesis that this span's data should be discarded.
// Closes the connection when the last span is closed.
func StopSpan(discard bool) {
	decrementSpanDepth()
	sendRequest("stop_span", map[string]any{"discard": discard})
	if getSpanDepth() == 0 {
		closeConnection()
	}
}

// Group runs a function within a labeled span and returns its result.
// This is the preferred way to group related generation calls.
func Group[T any](label uint64, fn func() T) T {
	StartSpan(label)
	result := fn()
	StopSpan(false)
	return result
}

// DiscardableGroup runs a function within a labeled span.
// If the function returns (value, false), the span is discarded.
// This is useful for filter-like operations where rejected values should be discarded.
func DiscardableGroup[T any](label uint64, fn func() (T, bool)) (T, bool) {
	StartSpan(label)
	result, ok := fn()
	StopSpan(!ok)
	return result, ok
}
