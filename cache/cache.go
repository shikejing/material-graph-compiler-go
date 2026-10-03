// Package cache 提供并发安全的编译结果缓存：单飞去重 + 有界 LRU。
package cache

import "sync"

// Cache 是并发安全的字符串键缓存，容量有界，重复计算被单飞合并。
type Cache struct {
	mu      sync.Mutex
	items   map[string]*entry
	lru     *lruList
	cap     int
	inflight map[string]*call
}

// New 创建容量为 capacity 的缓存（<=0 表示默认 256）。
func New(capacity int) *Cache {
	// TODO: 实现
	return nil
}

// GetOrCompute 返回键对应的值；未命中时调用 compute 且同一键并发只计算一次。
// 返回值要么是完整结果要么是错误，绝不返回半成品。
func (c *Cache) GetOrCompute(key string, compute func() (string, error)) (string, error) {
	// TODO: 实现
	return "", nil
}

// Len 返回当前缓存条目数。
func (c *Cache) Len() int { return 0 }
