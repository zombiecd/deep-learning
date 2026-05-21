# lab · 动手复现

五面墙里的每一个"我说"，这里都让你"亲眼见"。两类实验：**反汇编**（看原子操作编译成什么指令）和 **benchmark**（看竞争如何让性能崩塌）。只需要装好 Go（1.21+，因为用到泛型 `atomic.Pointer`）。

```
lab/
├── atomicops.go    # 反汇编材料：几个原子操作
├── bench_test.go   # C/D/E 三面墙的基准测试
└── go.mod
```

> ⚠️ 一句重要提醒：**数字会因机器、CPU 代次、甚至每次运行而浮动**。下面给的是一台 8 核（6 性能 + 2 能效）Apple M 系列上的代表值，重点看**趋势和数量级**（核越多越慢？差几倍？），不是抠到小数点。

---

## 实验一：原子操作编译成了什么（B 那面墙）

把同一段 Go 分别编译到两种架构，再反汇编对照。

```bash
cd topics/并发原理/lab

# 交叉编译出两个架构的二进制
GOARCH=amd64 GOOS=linux  go build -o /tmp/lab_amd64 .
GOARCH=arm64 GOOS=linux  go build -o /tmp/lab_arm64 .

# 反汇编单个函数（objdump 本身支持跨架构）
go tool objdump -s 'main\.doAdd'   /tmp/lab_amd64    # atomic.Add
go tool objdump -s 'main\.doStore' /tmp/lab_arm64    # atomic.Store
```

你会看到的关键行：

| 操作 | amd64 | arm64 |
|---|---|---|
| `doAdd` (atomic.Add) | `LOCK XADDQ`（单条） | LSE 单条 `LDADD` / 老芯片 `LDAXR…STLXR` 循环 |
| `doCAS` | `LOCK CMPXCHGQ` | 同上，LSE `CAS` / LL-SC 循环 |
| `doLoad` | 裸 `MOVQ`（无 LOCK） | `LDAR`（load-acquire 屏障） |
| `doStore` | `XCHGQ`（隐式全屏障） | `STLR`（store-release 屏障） |

**怎么读**：amd64 的 Load 是裸 `MOVQ`、不带屏障，而 arm64 的 Load 是带屏障的 `LDAR`——这就是"x86 强序、ARM 弱序，所以 Go 在 ARM 上要多插屏障"的一手证据（对应 A、B 两面墙）。而 `doAdd` 是单条指令，不是什么"CAS 自旋循环"。

---

## 实验二：竞争如何让性能崩（C / D / E 三面墙）

```bash
cd topics/并发原理/lab

# 全部 benchmark，对比 1 核 vs 8 核
go test -bench=. -benchtime=300ms -cpu=1,8

# 想看完整的"核越多越慢"曲线：
go test -bench='SharedCounter|ShardedCounter' -benchtime=300ms -cpu=1,2,4,8
```

> 🍎 **Apple Silicon / 交叉编译用户注意**：先看 `go env GOARCH`。如果它不是你的本机架构（比如设成了 `amd64` 而你在 M 系列 Mac 上），benchmark 会被交叉编译、走模拟器跑，数字严重失真。强制原生执行：
> ```bash
> GOARCH=arm64 GOOS=darwin go test -bench=. -benchtime=300ms -cpu=1,8   # Apple Silicon
> ```
> Intel/AMD 的 Linux/Mac 一般直接 `go test -bench=.` 即可。

### 代表性结果与读法

```
                          1核        8核
SharedCounter          7.2 ns     ~68 ns     ← C：抢同一条 cache line，核越多越慢
ShardedCounter         7.3 ns      2.6 ns     ← C：分片+padding，核越多越快(真并行)
FalseSharing          14.5 ns     ~49 ns     ← C：两个无关变量同住一条行，被迫互相拖累
Padded                14.5 ns     ~16 ns     ← C：padding 推开后几乎不退化
AtomicCounter          7.2 ns     ~62 ns     ┐
MutexCounter          14.1 ns    ~119 ns     ├ D：无竞争 atomic<Mutex<channel；
ChannelCounter        25.7 ns     ~72 ns     ┘    满竞争全崩，且都被 Sharded 暴打
ConfigRWMutex         14.5 ns    ~129 ns     ← E：读路径 readerCount 是热点，核越多越慢
ConfigAtomicPointer    6.9 ns      1.3 ns     ← E：COW 只读同一指针，核越多越快(~100x)
```

**三个该盯住的对照**：
1. `SharedCounter` 往上爬 vs `ShardedCounter` 往下掉——同一个加法，差别只在抢不抢同一条 cache line。
2. `FalseSharing` vs `Padded`——两个 goroutine 写的是**不同变量**，仅因同住一条 64 字节行就慢 3 倍。
3. `ConfigRWMutex` vs `ConfigAtomicPointer`——读多写少场景，COW 在 8 核上把 RWMutex 甩开约 100 倍。

读懂这三组对照，C、D、E 三面墙就从"听我说"变成了"我自己跑出来的"。

---

## 对应关系

| 实验 | 实证哪面墙 |
|---|---|
| 反汇编 doLoad/doStore 跨架构对比 | [A 内存模型](../01-内存模型.md) · [B 原子性](../02-原子性.md) |
| 反汇编 doAdd/doCAS | [B 原子性](../02-原子性.md) |
| Shared/Sharded/FalseSharing/Padded | [C 缓存一致性](../03-缓存一致性.md) |
| Atomic/Mutex/Channel Counter | [D 阻塞与调度](../04-阻塞与调度.md) |
| ConfigRWMutex/ConfigAtomicPointer | [E 范式之争](../05-范式之争.md) |
