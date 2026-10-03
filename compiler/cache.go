package compiler

import "sync"

// lruCache is a bounded, thread-safe cache with LRU eviction.
type lruCache[V any] struct {
	mu       sync.Mutex
	capacity int
	entries  map[string]V
	order    []string // oldest first
}

func newLRU[V any](capacity int) *lruCache[V] {
	if capacity <= 0 {
		capacity = 1
	}
	return &lruCache[V]{capacity: capacity, entries: map[string]V{}}
}

func (c *lruCache[V]) get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.entries[key]
	if !ok {
		var zero V
		return zero, false
	}
	// Move to end (most recently used).
	c.touch(key)
	return v, true
}

func (c *lruCache[V]) touch(key string) {
	for i, k := range c.order {
		if k == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			c.order = append(c.order, key)
			return
		}
	}
}

func (c *lruCache[V]) put(key string, v V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[key]; exists {
		c.entries[key] = v
		c.touch(key)
		return
	}
	c.entries[key] = v
	c.order = append(c.order, key)
	for len(c.order) > c.capacity {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.entries, oldest)
	}
}

func (c *lruCache[V]) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// ResultCache is a bounded, thread-safe cache of compiled results keyed by
// graph content hash and library version.
type ResultCache struct {
	cache *lruCache[*CompileResult]
}

// NewResultCache creates a cache with the given capacity.
func NewResultCache(capacity int) *ResultCache {
	return &ResultCache{cache: newLRU[*CompileResult](capacity)}
}

// Get returns the cached result for the key.
func (c *ResultCache) Get(key string) (*CompileResult, bool) {
	return c.cache.get(key)
}

// Put stores the result in the cache.
func (c *ResultCache) Put(key string, r *CompileResult) {
	c.cache.put(key, r)
}

// Len returns the number of cached entries.
func (c *ResultCache) Len() int { return c.cache.len() }

// FragmentCache is a bounded, thread-safe cache of node fragments keyed by
// content hash.
type FragmentCache struct {
	cache *lruCache[*Fragment]
}

// NewFragmentCache creates a fragment cache with the given capacity.
func NewFragmentCache(capacity int) *FragmentCache {
	return &FragmentCache{cache: newLRU[*Fragment](capacity)}
}

// Get returns the cached fragment for the key.
func (c *FragmentCache) Get(key string) (*Fragment, bool) {
	return c.cache.get(key)
}

// Put stores the fragment in the cache.
func (c *FragmentCache) Put(key string, f *Fragment) {
	c.cache.put(key, f)
}

// Len returns the number of cached fragments.
func (c *FragmentCache) Len() int { return c.cache.len() }
