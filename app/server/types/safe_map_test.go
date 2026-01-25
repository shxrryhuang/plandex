package types

import (
	"fmt"
	"sync"
	"testing"
)

// TestSafeMapBasicOperations tests basic CRUD operations on SafeMap
func TestSafeMapBasicOperations(t *testing.T) {
	t.Run("set and get", func(t *testing.T) {
		sm := NewSafeMap[int]()
		sm.Set("key1", 100)

		val := sm.Get("key1")
		if val != 100 {
			t.Errorf("expected value 100, got %d", val)
		}
	})

	t.Run("get nonexistent key returns zero value", func(t *testing.T) {
		sm := NewSafeMap[int]()

		val := sm.Get("nonexistent")
		if val != 0 {
			t.Errorf("expected zero value, got %d", val)
		}
	})

	t.Run("get nonexistent string returns empty string", func(t *testing.T) {
		sm := NewSafeMap[string]()

		val := sm.Get("nonexistent")
		if val != "" {
			t.Errorf("expected empty string, got %q", val)
		}
	})

	t.Run("delete existing key", func(t *testing.T) {
		sm := NewSafeMap[string]()
		sm.Set("key", "value")

		sm.Delete("key")

		val := sm.Get("key")
		if val != "" {
			t.Errorf("expected empty string after delete, got %q", val)
		}
	})

	t.Run("delete nonexistent key does not panic", func(t *testing.T) {
		sm := NewSafeMap[string]()

		// Should not panic
		sm.Delete("nonexistent")
	})

	t.Run("overwrite existing key", func(t *testing.T) {
		sm := NewSafeMap[int]()
		sm.Set("key", 1)
		sm.Set("key", 2)

		val := sm.Get("key")
		if val != 2 {
			t.Errorf("expected value 2 after overwrite, got %d", val)
		}
	})

	t.Run("length tracking", func(t *testing.T) {
		sm := NewSafeMap[int]()

		if sm.Len() != 0 {
			t.Errorf("expected length 0, got %d", sm.Len())
		}

		sm.Set("a", 1)
		sm.Set("b", 2)
		sm.Set("c", 3)

		if sm.Len() != 3 {
			t.Errorf("expected length 3, got %d", sm.Len())
		}

		sm.Delete("b")

		if sm.Len() != 2 {
			t.Errorf("expected length 2 after delete, got %d", sm.Len())
		}
	})

	t.Run("keys returns all keys", func(t *testing.T) {
		sm := NewSafeMap[int]()
		sm.Set("a", 1)
		sm.Set("b", 2)
		sm.Set("c", 3)

		keys := sm.Keys()
		if len(keys) != 3 {
			t.Errorf("expected 3 keys, got %d", len(keys))
		}

		keyMap := make(map[string]bool)
		for _, k := range keys {
			keyMap[k] = true
		}

		for _, expected := range []string{"a", "b", "c"} {
			if !keyMap[expected] {
				t.Errorf("expected key %q to be present", expected)
			}
		}
	})

	t.Run("items returns all items as a copy", func(t *testing.T) {
		sm := NewSafeMap[int]()
		sm.Set("a", 1)
		sm.Set("b", 2)

		items := sm.Items()
		if len(items) != 2 {
			t.Errorf("expected 2 items, got %d", len(items))
		}
		if items["a"] != 1 || items["b"] != 2 {
			t.Errorf("items mismatch: %v", items)
		}

		// Mutating the returned map must not affect the SafeMap.
		items["c"] = 3
		delete(items, "a")
		if sm.Len() != 2 {
			t.Errorf("expected SafeMap length 2 after mutating copy, got %d", sm.Len())
		}
		if sm.Get("a") != 1 {
			t.Errorf("expected SafeMap key 'a' to still be 1 after mutating copy")
		}
	})
}

// TestSafeMapUpdate tests the Update function
func TestSafeMapUpdate(t *testing.T) {
	t.Run("update existing key", func(t *testing.T) {
		type Counter struct {
			Value int
		}
		sm := NewSafeMap[*Counter]()
		sm.Set("counter", &Counter{Value: 10})

		sm.Update("counter", func(c *Counter) {
			c.Value += 5
		})

		val := sm.Get("counter")
		if val.Value != 15 {
			t.Errorf("expected value 15 after update, got %d", val.Value)
		}
	})

	t.Run("update nonexistent key does nothing", func(t *testing.T) {
		sm := NewSafeMap[int]()
		called := false

		sm.Update("nonexistent", func(v int) {
			called = true
		})

		if called {
			t.Error("update function should not be called for nonexistent key")
		}
	})
}

// TestSafeMapSetIfAbsent tests the atomic SetIfAbsent method
func TestSafeMapSetIfAbsent(t *testing.T) {
	t.Run("sets value when key is absent", func(t *testing.T) {
		sm := NewSafeMap[int]()

		existing, loaded := sm.SetIfAbsent("key", 42)
		if loaded {
			t.Error("expected loaded=false for absent key")
		}
		if existing != 0 {
			t.Errorf("expected zero value for absent key, got %d", existing)
		}
		if sm.Get("key") != 42 {
			t.Errorf("expected key to be set to 42, got %d", sm.Get("key"))
		}
	})

	t.Run("does not overwrite when key is present", func(t *testing.T) {
		sm := NewSafeMap[int]()
		sm.Set("key", 10)

		existing, loaded := sm.SetIfAbsent("key", 99)
		if !loaded {
			t.Error("expected loaded=true for present key")
		}
		if existing != 10 {
			t.Errorf("expected existing value 10, got %d", existing)
		}
		if sm.Get("key") != 10 {
			t.Errorf("expected key to remain 10, got %d", sm.Get("key"))
		}
	})

	t.Run("concurrent race: exactly one goroutine wins", func(t *testing.T) {
		sm := NewSafeMap[int]()
		numGoroutines := 100
		var wg sync.WaitGroup
		wins := make(chan int, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				_, loaded := sm.SetIfAbsent("race-key", id)
				if !loaded {
					wins <- id
				}
			}(i)
		}

		wg.Wait()
		close(wins)

		winCount := 0
		for range wins {
			winCount++
		}
		if winCount != 1 {
			t.Errorf("expected exactly 1 winner, got %d", winCount)
		}
	})
}

// TestSafeMapConcurrency tests thread-safety of SafeMap operations
func TestSafeMapConcurrency(t *testing.T) {
	t.Run("concurrent writes", func(t *testing.T) {
		sm := NewSafeMap[int]()
		var wg sync.WaitGroup
		numGoroutines := 100
		numOperations := 100

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < numOperations; j++ {
					key := fmt.Sprintf("key_%d_%d", id, j)
					sm.Set(key, j)
				}
			}(i)
		}

		wg.Wait()

		if sm.Len() != numGoroutines*numOperations {
			t.Errorf("expected %d entries, got %d", numGoroutines*numOperations, sm.Len())
		}
	})

	t.Run("concurrent reads and writes", func(t *testing.T) {
		sm := NewSafeMap[int]()
		sm.Set("counter", 0)

		var wg sync.WaitGroup
		numReaders := 50
		numWriters := 50
		numOperations := 100

		// Writers
		for i := 0; i < numWriters; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < numOperations; j++ {
					key := fmt.Sprintf("writer_%d", id)
					sm.Set(key, id*j)
				}
			}(i)
		}

		// Readers
		for i := 0; i < numReaders; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < numOperations; j++ {
					sm.Get("counter")
					sm.Len()
					sm.Keys()
				}
			}()
		}

		wg.Wait()
	})

	t.Run("concurrent deletes", func(t *testing.T) {
		sm := NewSafeMap[int]()
		numEntries := 1000

		// Pre-populate
		for i := 0; i < numEntries; i++ {
			sm.Set(fmt.Sprintf("key_%d", i), i)
		}

		var wg sync.WaitGroup
		numGoroutines := 10

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := id; j < numEntries; j += numGoroutines {
					sm.Delete(fmt.Sprintf("key_%d", j))
				}
			}(i)
		}

		wg.Wait()

		if sm.Len() != 0 {
			t.Errorf("expected 0 entries after deletion, got %d", sm.Len())
		}
	})

	t.Run("concurrent updates", func(t *testing.T) {
		type Counter struct {
			Value int
		}
		sm := NewSafeMap[*Counter]()
		sm.Set("shared", &Counter{Value: 0})

		var wg sync.WaitGroup
		numGoroutines := 100

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sm.Update("shared", func(c *Counter) {
					c.Value++
				})
			}()
		}

		wg.Wait()

		val := sm.Get("shared")
		if val.Value != numGoroutines {
			t.Errorf("expected value %d after concurrent updates, got %d", numGoroutines, val.Value)
		}
	})
}

// TestSafeMapEdgeCases tests edge cases and error conditions
func TestSafeMapEdgeCases(t *testing.T) {
	t.Run("empty string key", func(t *testing.T) {
		sm := NewSafeMap[int]()
		sm.Set("", 42)

		val := sm.Get("")
		if val != 42 {
			t.Errorf("failed to handle empty string key: got %d", val)
		}
	})

	t.Run("nil pointer value", func(t *testing.T) {
		sm := NewSafeMap[*int]()
		sm.Set("nil", nil)

		val := sm.Get("nil")
		if val != nil {
			t.Error("expected nil value")
		}
	})

	t.Run("special characters in key", func(t *testing.T) {
		sm := NewSafeMap[string]()
		specialKeys := []string{
			"key with spaces",
			"key\twith\ttabs",
			"key\nwith\nnewlines",
			"key/with/slashes",
			"key\\with\\backslashes",
			"key:with:colons",
			"日本語キー",
			"🔑emoji",
		}

		for _, key := range specialKeys {
			sm.Set(key, "value_"+key)
		}

		for _, key := range specialKeys {
			val := sm.Get(key)
			expected := "value_" + key
			if val != expected {
				t.Errorf("key %q: expected %q, got %q", key, expected, val)
			}
		}
	})

	t.Run("large number of entries", func(t *testing.T) {
		sm := NewSafeMap[int]()
		numEntries := 10000

		for i := 0; i < numEntries; i++ {
			sm.Set(fmt.Sprintf("key_%d", i), i*2)
		}

		if sm.Len() != numEntries {
			t.Errorf("expected %d entries, got %d", numEntries, sm.Len())
		}

		// Verify some entries
		for _, i := range []int{0, 1000, 5000, 9999} {
			val := sm.Get(fmt.Sprintf("key_%d", i))
			if val != i*2 {
				t.Errorf("entry %d: expected %d, got %d", i, i*2, val)
			}
		}
	})
}
