// atomicops.go —— B（原子性）的反汇编实验材料。
//
// 这几个函数本身没什么可跑的；它们的价值在于「编译出来长什么样」。
// 配合 README 里的 `go tool objdump` 命令，把同一个原子操作分别编译到
// amd64 和 arm64，亲眼对比生成的机器指令：
//   - amd64: atomic.Add → 单条 LOCK XADDQ；Load → 裸 MOVQ；Store → XCHGQ
//   - arm64: atomic.Add → 运行时分支(LSE 单条 LDADD / 老芯片 LL-SC 循环)；
//            Load → LDAR(屏障)；Store → STLR(屏障)
//
// //go:noinline 保证每个函数留作独立符号，objdump -s 才能单独定位它。
package main

import "sync/atomic"

var n int64

//go:noinline
func doAdd() { atomic.AddInt64(&n, 1) }

//go:noinline
func doCAS() { atomic.CompareAndSwapInt64(&n, 0, 1) }

//go:noinline
func doLoad() int64 { return atomic.LoadInt64(&n) }

//go:noinline
func doStore() { atomic.StoreInt64(&n, 5) }

func main() {
	doAdd()
	doCAS()
	_ = doLoad()
	doStore()
}
