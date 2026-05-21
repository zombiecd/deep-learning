// bench_test.go —— C/D/E 三面墙的可复现基准测试。
// 每组 benchmark 都标注了它实证的是哪面墙的哪个论断。
// 运行命令见同目录 README.md。
package main

import (
	"sync"
	"sync/atomic"
	"testing"
)

// ───────────────────────── C · 缓存一致性 ─────────────────────────

// 共享单计数器：所有 goroutine 抢同一条 cache line。
// 预期：核越多越慢（MESI 乒乓）。-cpu=1,2,4,8 看曲线往上爬。
var shared int64

func BenchmarkSharedCounter(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			atomic.AddInt64(&shared, 1)
		}
	})
}

// 分片计数器：每个 goroutine 写不同分片，且每片独占一条 cache line。
// 预期：核越多越快（真并行，无争用）。和上面形成对照。
const shards = 256

type paddedCell struct {
	v int64
	_ [56]byte // 8 + 56 = 64：占满一条 cache line，杜绝伪共享
}

var cells [shards]paddedCell
var idx int64

func BenchmarkShardedCounter(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		i := atomic.AddInt64(&idx, 1) % shards
		for pb.Next() {
			atomic.AddInt64(&cells[i].v, 1)
		}
	})
}

// 伪共享 vs padding：两个 goroutine 各写各的变量，但是否共享 cache line。
// 预期：紧挨着的明显更慢——明明零数据冲突，只因同住一条行。
type noPad struct{ a, b int64 }
type withPad struct {
	a int64
	_ [56]byte
	b int64
}

func BenchmarkFalseSharing(b *testing.B) {
	var s noPad
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); for i := 0; i < b.N; i++ { atomic.AddInt64(&s.a, 1) } }()
	go func() { defer wg.Done(); for i := 0; i < b.N; i++ { atomic.AddInt64(&s.b, 1) } }()
	wg.Wait()
}

func BenchmarkPadded(b *testing.B) {
	var s withPad
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); for i := 0; i < b.N; i++ { atomic.AddInt64(&s.a, 1) } }()
	go func() { defer wg.Done(); for i := 0; i < b.N; i++ { atomic.AddInt64(&s.b, 1) } }()
	wg.Wait()
}

// ───────────────────────── D · 阻塞与调度 ─────────────────────────

// 同一个「计数 +1」，三种同步方式，看代价差几个数量级。
// 预期(无竞争)：atomic < Mutex < channel，越"重"越贵。
var an int64

func BenchmarkAtomicCounter(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			atomic.AddInt64(&an, 1)
		}
	})
}

var mu sync.Mutex
var mn int64

func BenchmarkMutexCounter(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			mn++
			mu.Unlock()
		}
	})
}

// channel 当计数器（actor 模式）：发一个值，由 owner goroutine 累加。
// 这是反模式示范——简单状态高频同步别用 channel。
func BenchmarkChannelCounter(b *testing.B) {
	ch := make(chan int, 1024)
	done := make(chan int64)
	go func() {
		var n int64
		for range ch {
			n++
		}
		done <- n
	}()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ch <- 1
		}
	})
	b.StopTimer()
	close(ch)
	<-done
}

// ───────────────────────── E · 范式之争 ─────────────────────────

// 读多写少的配置读取：RWMutex 读路径 vs atomic.Pointer(COW)。
// 预期：RWMutex 核越多越慢(readerCount 是共享写、cache 热点)；
//       atomic.Pointer 核越多越快(同一指针只读、Shared 态无争用)。
type Config struct{ data map[string]string }

var rwConf = &Config{data: map[string]string{"k": "v"}}
var rwMu sync.RWMutex

func BenchmarkConfigRWMutex(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rwMu.RLock()
			_ = rwConf.data["k"]
			rwMu.RUnlock()
		}
	})
}

var atomicConf atomic.Pointer[Config]

func BenchmarkConfigAtomicPointer(b *testing.B) {
	atomicConf.Store(&Config{data: map[string]string{"k": "v"}})
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c := atomicConf.Load()
			_ = c.data["k"]
		}
	})
}
