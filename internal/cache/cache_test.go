package cache

import (
	"fmt"
	"testing"
	"time"
)

// TestLRUCacheBasicOps tests basic Set/Get operations
func TestLRUCacheBasicOps(t *testing.T) {
	cache := NewLRUCache[string, string](3)

	// Test Set and Get
	cache.Set("key1", "value1")
	val, ok := cache.Get("key1")
	if !ok || val != "value1" {
		t.Errorf("Expected value1, got %s (ok=%v)", val, ok)
	}

	// Test miss
	_, ok = cache.Get("nonexistent")
	if ok {
		t.Error("Expected cache miss")
	}
}

// TestLRUCacheEviction tests that LRU evicts oldest entries when capacity is exceeded
func TestLRUCacheEviction(t *testing.T) {
	cache := NewLRUCache[string, string](3)

	cache.Set("a", "1")
	cache.Set("b", "2")
	cache.Set("c", "3")

	// Cache is now full. Adding a 4th entry should evict "a" (oldest/LRU)
	cache.Set("d", "4")

	_, ok := cache.Get("a")
	if ok {
		t.Error("Expected 'a' to be evicted")
	}

	_, ok = cache.Get("d")
	if !ok {
		t.Error("Expected 'd' to be in cache")
	}
}

// TestLRUCachePromotion tests that accessing an entry promotes it to MRU
func TestLRUCachePromotion(t *testing.T) {
	cache := NewLRUCache[string, string](3)

	cache.Set("a", "1")
	cache.Set("b", "2")
	cache.Set("c", "3")

	// Access "a" to promote it to MRU (most recently used)
	cache.Get("a")

	// Add "d", should evict "b" (now the oldest)
	cache.Set("d", "4")

	_, ok := cache.Get("b")
	if ok {
		t.Error("Expected 'b' to be evicted after 'a' was promoted")
	}

	_, ok = cache.Get("a")
	if !ok {
		t.Error("Expected 'a' to still be in cache after promotion")
	}
}

// TestLRUCacheUpdate tests updating an existing key
func TestLRUCacheUpdate(t *testing.T) {
	cache := NewLRUCache[string, string](3)

	cache.Set("key1", "value1")
	val, ok := cache.Get("key1")
	if !ok || val != "value1" {
		t.Errorf("Expected value1, got %s", val)
	}

	cache.Set("key1", "value2")
	val, ok = cache.Get("key1")
	if !ok || val != "value2" {
		t.Errorf("Expected value2, got %s", val)
	}
}

// TestMLCacheEntry tests ML cache entries with TTL
func TestMLCacheEntry(t *testing.T) {
	cache := NewLRUCache[string, MLCacheEntry](100)

	futureTime := time.Now().Add(5 * time.Minute)
	entry := MLCacheEntry{
		IsMalicious: true,
		Category:    "phishing",
		Confidence:  0.95,
		ExpiresAt:   futureTime,
	}

	cache.Set("example.com", entry)

	retrieved, ok := cache.Get("example.com")
	if !ok {
		t.Error("Expected to retrieve ML cache entry")
	}

	if retrieved.Category != "phishing" || retrieved.Confidence != 0.95 {
		t.Errorf("Entry mismatch: got %+v", retrieved)
	}

	// Verify expiration time is set correctly
	if retrieved.ExpiresAt.Before(futureTime) || retrieved.ExpiresAt.After(futureTime.Add(1*time.Second)) {
		t.Errorf("Expected ExpiresAt around %v, got %v", futureTime, retrieved.ExpiresAt)
	}
}

// TestMLCacheSnapshot tests that snapshot returns all entries in order
func TestMLCacheSnapshot(t *testing.T) {
	cache := NewLRUCache[string, MLCacheEntry](5)

	entries := map[string]MLCacheEntry{
		"google.com": {
			IsMalicious: false,
			Category:    "safe",
			Confidence:  0.99,
			ExpiresAt:   time.Now().Add(5 * time.Minute),
		},
		"malware.xyz": {
			IsMalicious: true,
			Category:    "malware",
			Confidence:  0.92,
			ExpiresAt:   time.Now().Add(5 * time.Minute),
		},
		"phishing.net": {
			IsMalicious: true,
			Category:    "phishing",
			Confidence:  0.87,
			ExpiresAt:   time.Now().Add(5 * time.Minute),
		},
	}

	for domain, entry := range entries {
		cache.Set(domain, entry)
	}

	snapshot := cache.Snapshot()
	if len(snapshot) != len(entries) {
		t.Errorf("Expected %d entries in snapshot, got %d", len(entries), len(snapshot))
	}

	found := make(map[string]MLCacheEntry)
	for _, entry := range snapshot {
		found[entry.key] = entry.value
	}

	for domain, expectedEntry := range entries {
		gotEntry, exists := found[domain]
		if !exists {
			t.Errorf("Expected %s in snapshot", domain)
		}
		if gotEntry.Category != expectedEntry.Category {
			t.Errorf("Category mismatch for %s: expected %s, got %s", domain, expectedEntry.Category, gotEntry.Category)
		}
	}
}

// TestEvictExact tests removing a specific entry by key
func TestEvictExact(t *testing.T) {
	cache := NewLRUCache[string, string](5)

	cache.Set("a", "1")
	cache.Set("b", "2")
	cache.Set("c", "3")

	cache.EvictExact("b")

	_, ok := cache.Get("b")
	if ok {
		t.Error("Expected 'b' to be evicted")
	}

	_, ok = cache.Get("a")
	if !ok {
		t.Error("Expected 'a' to still be in cache")
	}

	_, ok = cache.Get("c")
	if !ok {
		t.Error("Expected 'c' to still be in cache")
	}
}

// TestCacheCapacity tests that cache respects max capacity
func TestCacheCapacity(t *testing.T) {
	capacity := 10
	cache := NewLRUCache[string, string](capacity)

	for i := 0; i < capacity*2; i++ {
		cache.Set(string(rune(i)), "value")
	}

	snapshot := cache.Snapshot()
	if len(snapshot) > capacity {
		t.Errorf("Cache exceeded capacity: expected at most %d, got %d", capacity, len(snapshot))
	}
}

// TestDomainNormalizationConsistency tests that different case variations of same domain are NOT conflated
// (This verifies the caller is responsible for normalization, not the cache)
func TestDomainNormalizationResponsibility(t *testing.T) {
	cache := NewLRUCache[string, string](100)

	// Cache stores whatever key is given - no normalization happens in cache
	cache.Set("Example.COM", "entry1")
	cache.Set("example.com", "entry2")

	// These are different keys in the cache
	val1, ok1 := cache.Get("Example.COM")
	val2, ok2 := cache.Get("example.com")

	if !ok1 || val1 != "entry1" {
		t.Error("Expected to retrieve exact key 'Example.COM'")
	}

	if !ok2 || val2 != "entry2" {
		t.Error("Expected to retrieve exact key 'example.com'")
	}

	// This confirms the caller (handleMLClassify, handleDNSQuery) must normalize
	// to ensure cache hits across different input cases
}

// TestLargeMLCacheLoad tests realistic scenario with 10000 ML entries
func TestLargeMLCacheLoad(t *testing.T) {
	cache := NewLRUCache[string, MLCacheEntry](10000)

	// Fill cache with 10000 entries
	for i := 0; i < 10000; i++ {
		domain := fmt.Sprintf("domain%d.test.com", i)
		entry := MLCacheEntry{
			IsMalicious: i%2 == 0,
			Category:    map[bool]string{true: "malicious", false: "safe"}[i%2 == 0],
			Confidence:  0.5 + float32(i%50)/100,
			ExpiresAt:   time.Now().Add(5 * time.Minute),
		}
		cache.Set(domain, entry)
	}

	snapshot := cache.Snapshot()
	if len(snapshot) != 10000 {
		t.Errorf("Expected 10000 entries, got %d", len(snapshot))
	}

	// Verify random entries are retrievable
	retrieved, ok := cache.Get("domain0.test.com")
	if !ok {
		t.Error("Expected to retrieve entry from large cache")
	}

	if retrieved.Category == "" {
		t.Error("Retrieved entry has empty category")
	}
}

// TestConcurrentCacheAccess tests that cache is safe for concurrent reads/writes
func TestConcurrentCacheAccess(t *testing.T) {
	cache := NewLRUCache[string, string](1000)
	done := make(chan bool, 20)
	errors := make(chan string, 20)

	// Concurrent writers
	for writer := 0; writer < 10; writer++ {
		go func(id int) {
			defer func() { done <- true }()
			for j := 0; j < 100; j++ {
				key := "key_" + string(rune(id)) + "_" + string(rune(j))
				cache.Set(key, "value")
			}
		}(writer)
	}

	// Concurrent readers
	for reader := 0; reader < 10; reader++ {
		go func(id int) {
			defer func() { done <- true }()
			for j := 0; j < 50; j++ {
				key := "key_" + string(rune(id%10)) + "_" + string(rune(j%100))
				_, _ = cache.Get(key)
			}
		}(reader)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 20; i++ {
		<-done
	}

	// Verify no errors and cache is not empty
	select {
	case err := <-errors:
		t.Fatalf("Concurrent access error: %s", err)
	default:
		// No errors
	}

	snapshot := cache.Snapshot()
	if len(snapshot) == 0 {
		t.Error("Expected entries in cache after concurrent access")
	}
}
