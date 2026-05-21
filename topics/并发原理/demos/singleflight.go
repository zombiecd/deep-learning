// singleflight.go —— 合并并发请求 / 防缓存击穿（对应 A·happens-before + D·锁粒度）。
//
// 同一个 key 的 N 个并发请求，只让第一个真正执行 fn，其余等待并共享结果。
// 缓存击穿（热点 key 失效瞬间大量请求同时穿透到 DB）的标准解法。
//
// 两个易错点，这里都处理了：
//   - 执行 fn 时不持锁（fn 可能慢，持锁会卡死其他所有 key）。
//   - fn panic 时也必须 Done，否则所有等待者永久阻塞——用 defer 兜住。
//
// 安全性的理论依据：WaitGroup.Done() happens-before Wait() 返回，
// 所以等待者读 c.val 时，第一个 goroutine 对 c.val 的写一定可见，无需额外加锁。
package demos

import (
	"fmt"
	"sync"
)

type call struct {
	wg  sync.WaitGroup
	val any
	err error
}

type Group struct {
	mu sync.Mutex
	m  map[string]*call
}

// Do 同一 key 并发调用时，fn 只执行一次，结果被所有调用方共享。
func (g *Group) Do(key string, fn func() (any, error)) (any, error) {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*call)
	}
	if c, ok := g.m[key]; ok {
		// 已有同 key 调用在途，等它、共享它的结果。
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}
	// 我是第一个：登记 call，wg.Add 必须在解锁前完成。
	c := new(call)
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	g.doCall(c, key, fn)
	return c.val, c.err
}

// doCall 在锁外执行 fn，并保证无论正常返回还是 panic，Done 与清理都会发生。
func (g *Group) doCall(c *call, key string, fn func() (any, error)) {
	defer func() {
		if r := recover(); r != nil {
			c.err = fmt.Errorf("singleflight: fn panic: %v", r)
		}
		c.wg.Done() // 唤醒所有等待者
		g.mu.Lock()
		delete(g.m, key) // 执行完移除，让下一波请求能重新触发 fn
		g.mu.Unlock()
	}()
	c.val, c.err = fn()
}
