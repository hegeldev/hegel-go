package hegel

import (
	"fmt"
	"sync"

	"hegel.dev/go/hegel/internal/libhegel"
)

// Pool is an engine-managed collection of values generated during a stateful
// test. It is safe to share between concurrent state-machine workers.
//
// A Pool belongs to the test case in which [NewPool] created it. Create a new
// pool with each new state-machine value; do not retain it between test cases.
type Pool[T any] struct {
	mu     sync.Mutex
	handle *libhegel.Pool
	values map[int64]T
}

// NewPool creates an empty value pool for a stateful test.
//
// Concurrent rules may share the returned pool. Operations that add or draw a
// value must receive the calling rule's TestCase, so each worker uses its own
// clone of the native test-case handle.
func NewPool[T any](tc TestCase) *Pool[T] {
	ctx, ltc := tc.engine()
	handle, err := ltc.NewPool(ctx)
	if err != nil {
		tc.abort(err)
		return nil
	}
	return &Pool[T]{handle: handle, values: make(map[int64]T)}
}

// Add registers value with the pool.
func (p *Pool[T]) Add(tc TestCase, value T) {
	p.mu.Lock()
	defer p.mu.Unlock()

	ctx, ltc := tc.engine()
	id, err := p.handle.Add(ctx, ltc)
	if err != nil {
		tc.abort(err)
		return
	}
	if _, exists := p.values[id]; exists {
		tc.abort(fmt.Errorf("pool: engine returned duplicate value ID %d", id))
		return
	}
	p.values[id] = value
}

// Len returns the number of values currently in the pool.
func (p *Pool[T]) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.values)
}

// IsEmpty reports whether the pool contains no values.
func (p *Pool[T]) IsEmpty() bool {
	return p.Len() == 0
}

// ValuesReusable returns a generator that draws values without removing them
// from the pool.
//
// Drawing from an empty pool rejects the current rule as though it called
// [TestCase.Assume] with false.
func (p *Pool[T]) ValuesReusable() Generator[T] {
	return poolGenerator[T]{pool: p}
}

// ValuesConsumed returns a generator that draws and removes values from the
// pool.
//
// Drawing from an empty pool rejects the current rule as though it called
// [TestCase.Assume] with false.
func (p *Pool[T]) ValuesConsumed() Generator[T] {
	return poolGenerator[T]{pool: p, consume: true}
}

type poolGenerator[T any] struct {
	pool    *Pool[T]
	consume bool
}

func (g poolGenerator[T]) draw(tc TestCase) (T, error) {
	var zero T
	g.pool.mu.Lock()
	defer g.pool.mu.Unlock()

	ctx, ltc := tc.engine()
	id, err := g.pool.handle.Generate(ctx, ltc, g.consume)
	if err != nil {
		return zero, err
	}
	value, exists := g.pool.values[id]
	if !exists {
		return zero, fmt.Errorf("pool: engine selected unknown value ID %d", id)
	}
	if g.consume {
		delete(g.pool.values, id)
	}
	return value, nil
}
