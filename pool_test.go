package hegel

import (
	"sync"
	"sync/atomic"
	"testing"

	"hegel.dev/go/hegel/internal/libhegel"
)

func TestNewPoolAbortsOnEngineError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t,
		uintptr(0), libhegel.E_BACKEND, "new pool boom",
	)
	defer expectErrorPanic(t, libhegel.E_BACKEND)
	NewPool[int](tc)
}

func TestPoolAddAbortsOnEngineError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t,
		uintptr(2), libhegel.OK,
		int64(0), libhegel.E_BACKEND, "pool add boom",
	)
	pool := NewPool[int](tc)
	defer expectErrorPanic(t, libhegel.E_BACKEND)
	pool.Add(tc, 42)
}

func TestPoolAddAbortsOnDuplicateEngineID(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t,
		uintptr(2), libhegel.OK,
		int64(7), libhegel.OK,
		int64(7), libhegel.OK,
	)
	pool := NewPool[int](tc)
	pool.Add(tc, 10)
	defer func() {
		r := recover()
		err, ok := r.(error)
		if !ok || err.Error() != "pool: engine returned duplicate value ID 7" {
			t.Errorf("panic = %v, want duplicate value ID error", r)
		}
	}()
	pool.Add(tc, 20)
}

func TestPoolDrawRejectsUnknownEngineID(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t,
		uintptr(2), libhegel.OK,
		int64(99), libhegel.OK,
	)
	pool := NewPool[int](tc)
	_, err := pool.ValuesReusable().draw(tc)
	if err == nil || err.Error() != "pool: engine selected unknown value ID 99" {
		t.Errorf("draw error = %v, want unknown value ID error", err)
	}
}

func TestPoolReusableAndConsumedValues(t *testing.T) {
	err := run(func(tc TestCase) {
		pool := NewPool[int](tc)
		if !pool.IsEmpty() || pool.Len() != 0 {
			tc.Errorf("new pool is not empty")
		}

		pool.Add(tc, 10)
		pool.Add(tc, 20)
		if pool.IsEmpty() || pool.Len() != 2 {
			tc.Errorf("pool length after Add = %d, want 2", pool.Len())
		}

		for range 10 {
			value := Draw(tc, pool.ValuesReusable())
			if value != 10 && value != 20 {
				tc.Errorf("reusable pool draw = %d, want 10 or 20", value)
			}
		}
		if pool.Len() != 2 {
			tc.Errorf("reusable draws changed pool length to %d", pool.Len())
		}

		first := Draw(tc, pool.ValuesConsumed())
		second := Draw(tc, pool.ValuesConsumed())
		if first == second {
			tc.Errorf("consumed the same value twice: %d", first)
		}
		if !pool.IsEmpty() {
			tc.Errorf("pool length after consuming all values = %d, want 0", pool.Len())
		}
	}, WithSingleTestCase())
	if err != nil {
		t.Fatalf("pool run: %v", err)
	}
}

type concurrentPoolMachine struct {
	pool     *Pool[int64]
	next     atomic.Int64
	consumed atomic.Int64
	mu       sync.Mutex
	seen     map[int64]struct{}
}

func (m *concurrentPoolMachine) RuleAdd(tc TestCase) {
	value := m.next.Add(1)
	m.pool.Add(tc, value)
}

func (m *concurrentPoolMachine) RuleConsume(tc TestCase) {
	value := Draw(tc, m.pool.ValuesConsumed())
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.seen[value]; exists {
		tc.Errorf("pool value %d was consumed twice", value)
	}
	m.seen[value] = struct{}{}
	m.consumed.Add(1)
}

func (m *concurrentPoolMachine) InvariantAccounting(tc TestCase) {
	want := m.next.Load() - m.consumed.Load()
	if got := int64(m.pool.Len()); got != want {
		tc.Errorf("pool contains %d values, want %d added but not consumed", got, want)
	}
}

func TestPoolIsSafeForConcurrentStateMachines(t *testing.T) {
	err := run(func(tc TestCase) {
		machine := &concurrentPoolMachine{
			pool: NewPool[int64](tc),
			seen: make(map[int64]struct{}),
		}
		RunStateful(tc, machine, WithParallelism(4))
	}, WithTestCases(20))
	if err != nil {
		t.Fatalf("concurrent pool run: %v", err)
	}
}
