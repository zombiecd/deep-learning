# demos · 真实业务里的并发代码

lab/ 是"原理解析"代码（反汇编、benchmark）；这里是**业务落地**代码——拿来就能用、或稍改就能用在你项目里的实现。每个都对应一面承重墙，把原理落到能跑的东西上。全部带测试，且通过 `-race` 数据竞争检测。

```bash
cd topics/concurrency/demos
go test -v -race ./...          # 跑全部（-race 验证无数据竞争）
# Apple Silicon 且 go env GOARCH 非 arm64 时，显式原生执行：
GOARCH=arm64 GOOS=darwin go test -v -race ./...
```

| 文件 | 是什么 | 真实场景 | 对应墙 |
|---|---|---|---|
| `config.go` | 配置热加载（COW + atomic.Pointer） | 配置中心推送 / 文件热更，海量 goroutine 无锁读 | [E](../05-paradigms.md) · [A](../01-memory-model.md) |
| `ratelimit.go` | 令牌桶 + 滑动窗口限流 | 接口限流、突发削峰 | [D](../04-blocking-scheduling.md) |
| `pool.go` | channel 对象池 | 数据库连接、长连接等限量资源复用 | [E](../05-paradigms.md) |
| `singleflight.go` | 合并并发请求 | 缓存击穿防护：热点 key 失效时只放一个请求重建 | [A](../01-memory-model.md) · [D](../04-blocking-scheduling.md) |

## 每个 demo 的一句话精髓

- **配置热加载**：不给配置加锁，而是让它不可变、用原子换指针。读路径完全无锁、多核越多越快（[实测](../lab/) 比 RWMutex 快百倍）。换指针后绝不再改旧/新对象——这是 COW 的纪律。
- **令牌桶**：惰性补算代替后台 ticker（少一个常驻 goroutine），float64 防低速率取整失真。功夫在把临界区压到只剩几步算术。
- **滑动窗口**：用时间戳队列消除固定窗口的"跨窗突刺"。队列天然有序，剔旧是 O(过期数)。
- **对象池**：`select + default` 实现非阻塞取还，并发安全由 channel 自带。**连接池不能用 `sync.Pool`**——GC 会清空它。
- **singleflight**：靠 `WaitGroup.Done` happens-before `Wait` 返回，等待者读结果不用加锁；fn 在锁外执行（不卡别的 key）；用 `defer + recover` 保证 panic 时也 `Done`，否则等待者永久阻塞。

## 怎么用它加深理解

别只读。建议：①先读对应那面墙的原理文档；②回来读这里的代码，对照"原理在代码哪一行落地"；③改坏它——比如把 `config.go` 的 `atomic.Pointer` 换成裸指针赋值，用 `-race` 跑，亲眼看竞争检测器报警；把 singleflight 的 `defer recover` 删掉，看 panic 怎么让等待者卡死。**把正确的代码改错、再看它怎么坏，是吃透原理最快的路。**
