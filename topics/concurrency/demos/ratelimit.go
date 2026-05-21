// ratelimit.go —— 令牌桶 + 滑动窗口限流器（对应 D·锁粒度）。
//
// 这两个限流器都用一把 Mutex，但真正的功夫在「把临界区压到最小」：
// 锁里只有几步算术，绝不碰 I/O、不开后台 goroutine。临界区越短，
// 竞争者要付的自旋/挂起代价就越小（见 04-blocking-scheduling）。
package demos

import (
	"sync"
	"time"
)

// TokenBucket 令牌桶：允许突发，长期受 rate 约束。
// 技巧：不开后台 ticker 定时加令牌，而是每次 Allow 时按"距上次过了多久 × 速率"惰性补算。
// 少一个常驻 goroutine，逻辑还更准。
type TokenBucket struct {
	mu       sync.Mutex
	capacity float64 // 桶容量（最大令牌数，也是突发上限）
	tokens   float64 // 当前令牌数；用 float64 是为了低速率下按比例补不失真
	rate     float64 // 每秒注入的令牌数
	last     time.Time
}

func NewTokenBucket(rate, capacity float64) *TokenBucket {
	return &TokenBucket{
		capacity: capacity,
		tokens:   capacity, // 初始装满
		rate:     rate,
		last:     time.Now(),
	}
}

// Allow 尝试取 1 个令牌。
func (b *TokenBucket) Allow() bool { return b.AllowN(1) }

// AllowN 尝试取 n 个令牌，成功扣减并返回 true，不足则拒绝。
func (b *TokenBucket) AllowN(n float64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	// 惰性补令牌：按时间差补，但不超过容量。
	b.tokens += now.Sub(b.last).Seconds() * b.rate
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
	b.last = now

	if b.tokens >= n {
		b.tokens -= n
		return true
	}
	return false
}

// SlidingWindow 滑动窗口（日志法）：记录窗口内每个请求的时间戳，
// 消除固定窗口"跨窗临界突刺"的问题（最后 100ms + 下一窗前 100ms 可放 2 倍）。
type SlidingWindow struct {
	mu     sync.Mutex
	window time.Duration
	limit  int
	times  []int64 // 请求时间戳队列（纳秒），按到达顺序天然有序
}

func NewSlidingWindow(window time.Duration, limit int) *SlidingWindow {
	return &SlidingWindow{
		window: window,
		limit:  limit,
		times:  make([]int64, 0, limit),
	}
}

// Allow 窗口内请求数未达上限则放行。
func (s *SlidingWindow) Allow() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UnixNano()
	boundary := now - int64(s.window)

	// 剔除窗口外的旧时间戳。队列有序，从头扫到第一个仍在窗口内的即可：O(过期数)。
	i := 0
	for i < len(s.times) && s.times[i] <= boundary {
		i++
	}
	s.times = s.times[i:]

	if len(s.times) < s.limit {
		s.times = append(s.times, now)
		return true
	}
	return false
}
