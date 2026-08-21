package hegel_test

import (
	"fmt"
	"maps"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"hegel.dev/go/hegel"
)

// concurrentKVStore is a tiny in-memory key-value store whose increment has a
// lost-update race: it reads the current value, releases the lock, and writes
// back the incremented value, so two workers incrementing the same key can
// each overwrite the other's update.
type concurrentKVStore struct {
	mu sync.Mutex
	m  map[int]int64
}

func newConcurrentKVStore() concurrentKVStore {
	return concurrentKVStore{m: make(map[int]int64)}
}

func (s *concurrentKVStore) get(key int) (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.m[key]
	return value, ok
}

func (s *concurrentKVStore) put(key int, value int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = value
}

func (s *concurrentKVStore) putIfAbsent(key int, value int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[key]; ok {
		return false
	}
	s.m[key] = value
	return true
}

func (s *concurrentKVStore) increment(key int) {
	value, _ := s.get(key)
	runtime.Gosched() // Make the lost-update race easier to find.
	s.put(key, value+1)
}

func (s *concurrentKVStore) snapshot() map[int]int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := make(map[int]int64, len(s.m))
	maps.Copy(snapshot, s.m)
	return snapshot
}

type concurrentKVTest struct {
	store      concurrentKVStore
	keys       *hegel.Pool[int]
	increments atomic.Int64
}

func (m *concurrentKVTest) RuleRegister(tc hegel.TestCase) {
	key := hegel.Draw(tc, hegel.Integers(0, 3))
	if m.store.putIfAbsent(key, 0) {
		m.keys.Add(tc, key)
	}
}

func (m *concurrentKVTest) RuleIncrement(tc hegel.TestCase) {
	key := hegel.Draw(tc, m.keys.ValuesReusable())
	m.store.increment(key)
	m.increments.Add(1)
}

func (m *concurrentKVTest) RuleRead(tc hegel.TestCase) {
	key := hegel.Draw(tc, m.keys.ValuesReusable())
	value, ok := m.store.get(key)
	tc.Note(fmt.Sprintf("read %d -> %d (present: %t)", key, value, ok))
}

func (m *concurrentKVTest) RuleSnapshot(tc hegel.TestCase) {
	tc.Note(fmt.Sprintf("snapshot holds %d keys", len(m.store.snapshot())))
}

func (m *concurrentKVTest) InvariantNoLostUpdates(tc hegel.TestCase) {
	var stored int64
	for _, value := range m.store.snapshot() {
		stored += value
	}
	performed := m.increments.Load()
	if stored != performed {
		tc.Errorf(
			"increments were lost: the store sums to %d after %d increments",
			stored,
			performed,
		)
	}
}

func ExampleRunStateful_concurrent() {
	hegel.Test(new(testing.T), func(tc *hegel.T) {
		machine := &concurrentKVTest{
			store: newConcurrentKVStore(),
			keys:  hegel.NewPool[int](tc),
		}
		hegel.RunStateful(
			tc,
			machine,
			hegel.WithBoundedConcurrency(4),
			hegel.WithRuleGroup("operations", "RuleRegister", "RuleIncrement", "RuleRead"),
			hegel.WithRuleGroup("snapshot", "RuleSnapshot"),
		)
	})
}
