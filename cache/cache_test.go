package cache_test

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shikejing/material-graph-compiler-go/cache"
)

func TestConcurrentSingleComputation(t *testing.T) {
	c := cache.New[string](16)
	var calls int64
	const workers = 64
	var wg sync.WaitGroup
	results := make([]string, workers)
	barrier := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-barrier // 同时放行，最大化并发竞争
			v, err := c.GetOrCompute("key", func() (string, error) {
				atomic.AddInt64(&calls, 1)
				time.Sleep(5 * time.Millisecond)
				return "value", nil
			})
			if err != nil {
				t.Error(err)
				return
			}
			results[i] = v
		}(i)
	}
	close(barrier)
	wg.Wait()
	if got := atomic.LoadInt64(&calls); got != 1 {
		t.Fatalf("compute ran %d times, want 1", got)
	}
	for i, v := range results {
		if v != "value" {
			t.Fatalf("worker %d got %q", i, v)
		}
	}
}

func TestBoundedGrowth(t *testing.T) {
	c := cache.New[string](50)
	for i := 0; i < 1000; i++ {
		_, err := c.GetOrCompute(fmt.Sprintf("k%d", i), func() (string, error) {
			return "v", nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if c.Len() > 50 {
		t.Fatalf("cache grew beyond capacity: %d", c.Len())
	}
	// LRU：最早一批键应被淘汰，最近的键应保留。
	if _, ok := c.Peek("k0"); ok {
		t.Fatal("oldest key should have been evicted")
	}
	if _, ok := c.Peek("k999"); !ok {
		t.Fatal("newest key should be present")
	}
}

func TestErrorNotCached(t *testing.T) {
	c := cache.New[string](8)
	var calls int64
	compute := func() (string, error) {
		atomic.AddInt64(&calls, 1)
		return "", errors.New("boom")
	}
	for i := 0; i < 3; i++ {
		if _, err := c.GetOrCompute("k", compute); err == nil {
			t.Fatal("expected error")
		}
	}
	if calls != 3 {
		t.Fatalf("failed computations must not be cached, calls=%d", calls)
	}
	if c.Len() != 0 {
		t.Fatalf("failed results must not occupy cache, len=%d", c.Len())
	}
}

func TestNoHalfBakedReads(t *testing.T) {
	// 慢计算期间，所有读者要么等到完整结果，要么拿到错误；不得读到零值半成品。
	c := cache.New[string](4)
	const workers = 32
	var wg sync.WaitGroup
	started := make(chan struct{})
	go func() {
		close(started)
		c.GetOrCompute("k", func() (string, error) {
			time.Sleep(20 * time.Millisecond)
			return "full", nil
		})
	}()
	<-started
	time.Sleep(time.Millisecond) // 确保计算已开始
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := c.GetOrCompute("k", func() (string, error) {
				return "duplicate", nil
			})
			if err != nil {
				return
			}
			if v != "full" {
				t.Errorf("got half-baked or duplicate value %q", v)
			}
		}()
	}
	wg.Wait()
}
