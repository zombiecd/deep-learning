// pool.go —— 基于 channel 的对象池（对应 E·范式）。
//
// channel 用在这里是它的"主场"：池的本质就是资源所有权在借用方和池之间转移，
// 正是 CSP 的拿手好戏。缓冲容量天然就是池容量，取=recv、还=send，
// 并发安全由 channel 自带——一行锁都不用写。
//
// 为什么不用 sync.Pool？因为 GC 会清空 sync.Pool。数据库连接、长连接这种
// "昂贵、需限量、要常驻"的资源，必须自己掌控生命周期。
package demos

import "sync"

type Pool struct {
	ch      chan any
	factory func() any
	mu      sync.Mutex
	closed  bool
}

// New 建一个容量为 size 的池，factory 在池空时创建新对象。
func New(size int, factory func() any) *Pool {
	return &Pool{
		ch:      make(chan any, size),
		factory: factory,
	}
}

// Get 取一个对象：池里有就复用，没有就新建（非阻塞策略）。
// select+default 是用 channel 做池的灵魂——靠 default 分支避免空池时傻等。
func (p *Pool) Get() any {
	select {
	case obj := <-p.ch:
		return obj
	default:
		return p.factory()
	}
}

// Put 归还对象：池没满就放回，满了就丢弃交给 GC（非阻塞）。
// 注意：复用前对象可能有脏数据，调用方需自行 Reset。
func (p *Pool) Put(obj any) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()

	select {
	case p.ch <- obj:
	default: // 池满，丢弃
	}
}

// Close 关闭池，幂等。
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	close(p.ch)
	for range p.ch { // 排空，让对象可被 GC（如需清理在此做）
	}
}
