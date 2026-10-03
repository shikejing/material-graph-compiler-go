package cache

// entry 是缓存中的一个条目。
type entry struct {
	key   string
	value string
	// LRU 链表指针
	prev, next *entry
}

// lruList 是双向链表，头部为最近使用。
type lruList struct {
	head, tail *entry
}

// call 表示一次进行中的计算（单飞）。
type call struct {
	done chan struct{}
	val  string
	err  error
}
