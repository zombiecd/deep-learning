package demos

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 配置热加载：读到初始值；Reload 后读到新值；旧快照不被污染。
func TestConfigHotReload(t *testing.T) {
	m := NewManager(map[string]string{"timeout": "3s"})

	old := m.Current() // 拿一个旧快照
	if v, _ := old.Get("timeout"); v != "3s" {
		t.Fatalf("want 3s, got %q", v)
	}

	m.Reload(map[string]string{"timeout": "5s"})

	if v, _ := m.Current().Get("timeout"); v != "5s" {
		t.Fatalf("reload 后应为 5s, got %q", v)
	}
	// 关键：旧快照仍是 3s，没被热更新污染——读者读的是一致快照。
	if v, _ := old.Get("timeout"); v != "3s" {
		t.Fatalf("旧快照应保持 3s, got %q", v)
	}
}

// 令牌桶：满桶可连放 cap 个，第 cap+1 个被拒；等待后补回。
func TestTokenBucket(t *testing.T) {
	b := NewTokenBucket(1000, 5) // 1000/s，容量 5

	for i := 0; i < 5; i++ {
		if !b.Allow() {
			t.Fatalf("第 %d 个应放行", i+1)
		}
	}
	if b.Allow() {
		t.Fatal("第 6 个应被拒（桶已空）")
	}
	time.Sleep(10 * time.Millisecond) // 1000/s 下足够补回若干令牌
	if !b.Allow() {
		t.Fatal("等待补令牌后应放行")
	}
}

// 滑动窗口：窗口内放够 limit 个后拒绝。
func TestSlidingWindow(t *testing.T) {
	s := NewSlidingWindow(time.Second, 3)
	for i := 0; i < 3; i++ {
		if !s.Allow() {
			t.Fatalf("第 %d 个应放行", i+1)
		}
	}
	if s.Allow() {
		t.Fatal("第 4 个应被拒")
	}
}

// 对象池：Put 进去的对象会被后续 Get 复用。
func TestPool(t *testing.T) {
	var created int64
	p := New(2, func() any {
		atomic.AddInt64(&created, 1)
		return new([1024]byte)
	})

	a := p.Get() // 池空 → 新建（created=1）
	p.Put(a)
	b := p.Get() // 池里有 → 复用，不新建
	if b != a {
		t.Fatal("应复用同一个对象")
	}
	if created != 1 {
		t.Fatalf("应只新建 1 次, got %d", created)
	}
	p.Close()
}

// singleflight：同 key 的并发调用，fn 只执行一次，结果共享。
func TestSingleflight(t *testing.T) {
	var g Group
	var calls int64

	const n = 100
	var wg sync.WaitGroup
	results := make([]any, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			v, _ := g.Do("hot-key", func() (any, error) {
				atomic.AddInt64(&calls, 1)
				time.Sleep(20 * time.Millisecond) // 模拟慢查询，让并发真正撞上
				return "value", nil
			})
			results[idx] = v
		}(i)
	}
	wg.Wait()

	if calls != 1 {
		t.Fatalf("fn 应只执行 1 次, got %d", calls)
	}
	for i, r := range results {
		if r != "value" {
			t.Fatalf("第 %d 个结果应共享为 value, got %v", i, r)
		}
	}
}

// singleflight：fn panic 不应让等待者永久阻塞，而是转成 error 返回。
func TestSingleflightPanic(t *testing.T) {
	var g Group
	done := make(chan struct{})
	go func() {
		_, err := g.Do("boom", func() (any, error) {
			panic("explode")
		})
		if err == nil {
			t.Error("panic 应转成 error")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("panic 导致永久阻塞——Done 没被调用")
	}
}
