# 主题：并发的第一性原理

一条脊梁：**并发安全 = 原子性 + 可见性**。所有同步原语（锁、atomic、channel、WaitGroup、Once）真正干的唯一一件事，就是在执行流之间建立 **happens-before** 关系，从而同时给出原子性与可见性。

但这条脊梁还有更深的一层：**底层是分层的，越往下越不分语言**。同一套硅（CPU 缓存一致性、原子指令、硬件乱序）撑起所有语言的并发；各语言的差异，是在硅层之上**选择暴露多少乱序自由**——这不是语法糖，是每个语言在「安全 ↔ 控制」上的世界观。所以本主题用 **Go 当主镜头**，关键处横向对照 C++ / Rust / Java，把通用的部分和 Go 特有的部分分清楚。

**原理 + 实践**：A–E 五篇讲透原理；[`lab/`](./lab/) 是可复现的反汇编与 benchmark；[`demos/`](./demos/) 是拿来就能用的业务代码（配置热加载、限流、对象池、singleflight）。**只想理解并发原理，顺序读 A–E 即可**；想动手验证进 lab；想看原理怎么落到业务代码进 demos；想做面试复习看下面那张映射表。

> 选题取材于一次 Go 底层原理面试复盘。但复盘材料是参考不是源——里头有「结论对、机理错」的地方（勘误见下），本主题逐条回一手资料 / 反汇编 / benchmark 重讲，不靠"听起来对"。

---

## 五面承重墙（问题脊柱）

每道题都问"为什么必须这样"，都藏一个反直觉的 puzzle，都能用反汇编 / litmus / benchmark 验证。

### A — 内存模型 ＝ 语言的世界观
我明明按顺序写了 `a=1; b=2`，凭什么编译器和 CPU 有权重排？为什么单线程从没出过事，一上多线程就崩？同一个硅层乱序，凭什么 **Go 只给"顺序一致"一个档**、**C++ 给六档 `memory_order`**、**Rust 抄 C++ 六档却让你用错就编译不过**、**Java `volatile` 卡在中间**？"上层包装"到底包了什么？
> 这是整面墙的承重柱：happens-before 的正身。

### B — 原子性的硅层
`counter++` 不原子，凭什么 `atomic.Add` 一条指令就原子？`LOCK`（x86）/ `LDXR-STXR`（ARM）锁的是什么——总线还是缓存？为什么同一个 `atomic.Store`，在 arm64 Mac 上要多花 `LDAR/STLR` 屏障、在 amd64 上裸 `MOV` 就够，而 Go 让你两台机器都不用操心？

### C — 为什么"核越多越慢"
一个原子计数器，CPU 核越多反而越慢，为什么？`atomic.Add` 又不自旋，那它慢在哪？MESI 缓存乒乓 + 伪共享——为什么这是**所有语言共有的墙**（Java `LongAdder`、Go `sync.Pool`、C++ padding 是同一招）。

### D — 抢不到锁之后
"锁无竞争时就是一次 CAS、几乎免费"——真的零成本吗？竞争升高时代价怎么一段段叠上去（自旋 → 挂起 → 饥饿模式）？park/wake 一个等待者：OS 线程走 futex vs **Go 把 goroutine 挂在用户态 runtime** vs Java 21 虚拟线程绕回 Go 的 M:N 模型。绿色线程为什么死过一次又复活？

### E — 范式之争
共享内存 + 锁 vs CSP / channel vs actor vs async-await。channel 底层既然是 Mutex + 环形队列，"用通信代替共享内存"到底买到了什么、代价是什么？什么时候它是性能反模式？
> 收口题。

---

## 面试复习索引：12 道高频题 → 承重墙映射

> 这一节是给**面试复习**用的索引，不是学原理的必经之路。3 道主问 + 5 道延伸 + 4 道手写，全是上面五面墙的推论——顺着墙看，不要背孤卡。

| 高频题 | 主要落在 | 纠正 / 钻深的点 |
|---|---|---|
| 主1 配置热加载并发安全 | A · E · D | 为什么最优解是「不可变 + 换指针」而非锁；`atomic.Pointer` 靠的是 happens-before 不是"原子读指针" |
| 主2 锁的本质 | B · D | 无竞争退化成一次 CAS；挂起用的是 **runtime semaphore 不是直接 OS futex** |
| 主3 原子为何轻 + 高竞争 | B · C | **`atomic.Add` 是单条 `LOCK XADD`，不是 CAS 自旋**（勘误见下）；高竞争崩在 cache 乒乓不在自旋 |
| 延1 RWMutex 何时更慢 | C · D | `readerCount` 多核 cache 争用；写饥饿 |
| 延2 sync.Map 为何读快 | A · C | read 字段是 `atomic.Pointer`，把"频繁小同步"摊销成"偶尔大同步" |
| 延3 sync.Pool per-P | C · D | 分片消除竞争；为什么绑 P 不绑 goroutine；GC 会清空 |
| 延4 channel 底层有锁吗 | D · E | 优雅 ≠ 无锁 ≠ 快；高频计数用 channel 是反模式 |
| 延5 happens-before | A | 整套题的第一性原理，A 的正身 |
| 手1 令牌桶 | D | 锁粒度 + 惰性补算 |
| 手2 滑动窗口 | D | 固定窗口临界突刺 |
| 手3 对象池（channel） | E · D | channel 当池；连接池不能用 sync.Pool |
| 手4 singleflight | A · D | `WaitGroup.Done` happens-before `Wait` 返回，所以读结果不用再加锁 |

---

## 已验证的勘误（纠正一个流行说法）

- **`atomic.AddInt64` 不是 CAS 自旋循环**。amd64 上是单条 `LOCK XADDQ`；arm64 上 Go 按 CPU 特性做**运行时分支**——支持 LSE 的芯片（如 Apple M 系列）走单条 `LDADDAL`，老 ARM 才回退到 `LDAXR/STLXR` 的 LL/SC 循环。"它是 CAS 自旋"是把抽象当实现的类别错误：具体指令随 ISA、甚至随 CPU 代次变。由此"高竞争原子操作崩在自旋"这个流行归因也站不住——真正瓶颈是 C 那面墙的 cache line 乒乓。（[lab/](./lab/) 可一手反汇编复现）

---

## 状态

| 墙 | 文档 | 状态 |
|---|---|---|
| A 内存模型 | [`01-内存模型.md`](./01-内存模型.md) | ✅ 已成文 |
| B 原子性硅层 | [`02-原子性.md`](./02-原子性.md) | ✅ 已成文 |
| C 核越多越慢 | [`03-缓存一致性.md`](./03-缓存一致性.md) | ✅ 已成文 |
| D 抢不到锁之后 | [`04-阻塞与调度.md`](./04-阻塞与调度.md) | ✅ 已成文 |
| E 范式之争 | [`05-范式之争.md`](./05-范式之争.md) | ✅ 已成文 |
| 实践 lab | [`lab/`](./lab/) | ✅ 反汇编 + benchmark，可亲手复现 |
| 业务 demos | [`demos/`](./demos/) | ✅ 配置热加载/限流/对象池/singleflight，带测试过 -race |

## 延伸阅读（一手）
- [Go Memory Model](https://go.dev/ref/mem)
- [Rustonomicon: Races](https://doc.rust-lang.org/nomicon/races.html) / [Send and Sync](https://doc.rust-lang.org/nomicon/send-and-sync.html)
- [JEP 444: Virtual Threads](https://openjdk.org/jeps/444)
