package cache

// entry 是缓存中的一个条目。
type entry[T any] struct {
	key   string
	value T
	prev, next *entry[T]
}

// lruList 是双向链表，头部为最近使用。
type lruList[T any] struct {
	head, tail *entry[T]
}

func (l *lruList[T]) pushFront(e *entry[T]) {
	e.prev = nil
	e.next = l.head
	if l.head != nil {
		l.head.prev = e
	}
	l.head = e
	if l.tail == nil {
		l.tail = e
	}
}

func (l *lruList[T]) remove(e *entry[T]) {
	if e.prev != nil {
		e.prev.next = e.next
	} else {
		l.head = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		l.tail = e.prev
	}
	e.prev, e.next = nil, nil
}

func (l *lruList[T]) moveToFront(e *entry[T]) {
	if l.head == e {
		return
	}
	l.remove(e)
	l.pushFront(e)
}

// call 表示一次进行中的计算（单飞）。
type call[T any] struct {
	done chan struct{}
	val  T
	err  error
}
