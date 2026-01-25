package types

import "sync"

type SafeMap[V any] struct {
	items map[string]V
	mu    sync.Mutex
}

func NewSafeMap[V any]() *SafeMap[V] {
	return &SafeMap[V]{items: make(map[string]V)}
}

func (sm *SafeMap[V]) Get(key string) V {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.items[key]
}

func (sm *SafeMap[V]) Set(key string, value V) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.items[key] = value
}

func (sm *SafeMap[V]) Delete(key string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.items, key)
}

func (sm *SafeMap[V]) Update(key string, fn func(V)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if item, ok := sm.items[key]; ok {
		fn(item)
		sm.items[key] = item
	}
}

// Items returns a shallow copy of the map. The returned map is safe to
// iterate and mutate without holding the SafeMap's lock.
func (sm *SafeMap[V]) Items() map[string]V {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	cp := make(map[string]V, len(sm.items))
	for k, v := range sm.items {
		cp[k] = v
	}
	return cp
}

// SetIfAbsent atomically checks whether key already exists. If absent it
// stores value and returns (zeroValue, false). If present it leaves the
// existing entry untouched and returns (existingValue, true).
func (sm *SafeMap[V]) SetIfAbsent(key string, value V) (V, bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if existing, ok := sm.items[key]; ok {
		return existing, true
	}
	sm.items[key] = value
	var zero V
	return zero, false
}

func (sm *SafeMap[V]) Keys() []string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	keys := make([]string, len(sm.items))
	i := 0
	for k := range sm.items {
		keys[i] = k
		i++
	}
	return keys
}

func (sm *SafeMap[V]) Len() int {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return len(sm.items)
}
