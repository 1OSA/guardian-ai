package cache

import (
	"container/list"
	"sync"
	"time"

	"guardian-ai/internal/config"

	"github.com/miekg/dns"
)

// LogLevel represents the logging verbosity level.
type LogLevel int

const (
	LogLevelError LogLevel = iota
	LogLevelWarn
	LogLevelInfo
	LogLevelDebug
)

var LogLevelNames = [...]string{"ERROR", "WARN", "INFO", "DEBUG"}

// dnsCacheEntry holds a cached DNS response.
type DNSCacheEntry struct {
	Msg       *dns.Msg
	ExpiresAt time.Time
}

// LRUCache is a generic O(1) LRU cache backed by a doubly-linked list + map.
// The zero value is not usable; use NewLRUCache.
type LRUCache[K comparable, V any] struct {
	cap   int
	mu    sync.Mutex
	items map[K]*list.Element
	order *list.List // front = LRU (oldest), back = MRU (newest)
}

type lruEntry[K comparable, V any] struct {
	key   K
	value V
}

// NewLRUCache creates a new LRU cache with the specified capacity.
func NewLRUCache[K comparable, V any](capacity int) *LRUCache[K, V] {
	return &LRUCache[K, V]{
		cap:   capacity,
		items: make(map[K]*list.Element, capacity),
		order: list.New(),
	}
}

// Get retrieves a value and promotes it to MRU. Returns zero-value + false on miss.
func (c *LRUCache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}
	c.order.MoveToBack(el)
	return el.Value.(*lruEntry[K, V]).value, true
}

// Set inserts or updates a value, evicting the LRU entry when over capacity.
func (c *LRUCache[K, V]) Set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		el.Value.(*lruEntry[K, V]).value = value
		c.order.MoveToBack(el)
		return
	}
	if c.order.Len() >= c.cap {
		if front := c.order.Front(); front != nil {
			c.order.Remove(front)
			delete(c.items, front.Value.(*lruEntry[K, V]).key)
		}
	}
	el := c.order.PushBack(&lruEntry[K, V]{key: key, value: value})
	c.items[key] = el
}

// Snapshot returns a copy of all entries (used for DB persistence).
func (c *LRUCache[K, V]) Snapshot() []lruEntry[K, V] {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]lruEntry[K, V], 0, c.order.Len())
	for el := c.order.Front(); el != nil; el = el.Next() {
		out = append(out, *el.Value.(*lruEntry[K, V]))
	}
	return out
}

// EvictExact removes a single entry by exact key.
// Must be called without c.mu held.
func (c *LRUCache[K, V]) EvictExact(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.order.Remove(el)
		delete(c.items, key)
	}
}

// ── Token-bucket rate limiter ─────────────────────────────────────────────────

// TokenBucket implements a token-bucket rate limiter.
type TokenBucket struct {
	tokens    float64
	lastRefil time.Time
}

// Allow returns true if the request should be permitted and deducts one token.
func (tb *TokenBucket) Allow() bool {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefil).Seconds()
	tb.tokens += elapsed * (float64(config.RateLimitPerMin) / 60.0)
	if tb.tokens > float64(config.RateLimitBurst) {
		tb.tokens = float64(config.RateLimitBurst)
	}
	tb.lastRefil = now
	if tb.tokens < 1 {
		return false
	}
	tb.tokens--
	return true
}

// MLCacheEntry holds cached ML classification results.
type MLCacheEntry struct {
	IsMalicious bool
	Category    string
	Confidence  float32
	ExpiresAt   time.Time
}
