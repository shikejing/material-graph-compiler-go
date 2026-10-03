// Package cache 提供并发安全的编译结果缓存：单飞去重 + 有界 LRU。
//
// 保证：
//   - 同一键在并发请求下只计算一次（单飞合并）；
//   - 读者要么拿到完整结果要么拿到错误，绝不返回半成品；
//   - 条目数不超过容量上限，淘汰最近最少使用项，占用不随请求量无界增长。
package cache

import "sync"

// Cache 是并发安全的字符串键缓存，值类型为 T。
type Cache[T any] struct {
	mu       sync.Mutex
	items    map[string]*entry[T]
	lru      *lruList[T]
	cap      int
	inflight map[string]*call[T]
}

// New 创建容量为 capacity 的缓存（<=0 表示默认 256）。
func New[T any](capacity int) *Cache[T] {
	if capacity <= 0 {
		capacity = 256
	}
	return &Cache[T]{
		items:    make(map[string]*entry[T]),
		lru:      &lruList[T]{},
		cap:      capacity,
		inflight: make(map[string]*call[T]),
	}
}

// GetOrCompute 返回键对应的值；未命中时调用 compute 且同一键并发只计算一次。
// compute 失败的结果不写入缓存（失败可重试），但会分发给当前等待者。
func (c *Cache[T]) GetOrCompute(key string, compute func() (T, error)) (T, error) {
	c.mu.Lock()
	if e, ok := c.items[key]; ok {
		c.lru.moveToFront(e)
		val := e.value
		c.mu.Unlock()
		return val, nil
	}
	if cl, ok := c.inflight[key]; ok {
		// 已有相同键的计算在进行：等待其完成。
		c.mu.Unlock()
		<-cl.done
		return cl.val, cl.err
	}
	cl := &call[T]{done: make(chan struct{})}
	c.inflight[key] = cl
	c.mu.Unlock()

	val, err := compute()

	c.mu.Lock()
	delete(c.inflight, key)
	cl.val, cl.err = val, err
	close(cl.done)
	if err == nil {
		c.putLocked(key, val)
	}
	c.mu.Unlock()
	return val, err
}

// Peek 只读查询缓存，不触发计算，不更新 LRU（用于判断命中来源）。
func (c *Cache[T]) Peek(key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[key]; ok {
		return e.value, true
	}
	var zero T
	return zero, false
}

// putLocked 写入缓存并在超容时淘汰 LRU 尾部。调用方须持有锁。
func (c *Cache[T]) putLocked(key string, val T) {
	if e, ok := c.items[key]; ok {
		e.value = val
		c.lru.moveToFront(e)
		return
	}
	e := &entry[T]{key: key, value: val}
	c.items[key] = e
	c.lru.pushFront(e)
	for len(c.items) > c.cap {
		tail := c.lru.tail
		if tail == nil {
			break
		}
		c.lru.remove(tail)
		delete(c.items, tail.key)
	}
}

// Invalidate 删除指定键（若存在）。
func (c *Cache[T]) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[key]; ok {
		c.lru.remove(e)
		delete(c.items, key)
	}
}

// Len 返回当前缓存条目数。
func (c *Cache[T]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}
