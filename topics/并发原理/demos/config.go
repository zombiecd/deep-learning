// config.go —— 配置热加载（对应 E·范式 / A·内存模型）。
//
// 真实场景：服务启动时加载一份配置，运行中由文件变更或配置中心推送触发热更新，
// 而成千上万业务 goroutine 每个请求都在读它。读极多、写极少。
//
// 最优解不是给配置加锁，而是 COW（Copy-On-Write）：配置对象不可变，
// 更新时整体造一份新的、用 atomic.Pointer 原子换过去。读路径完全无锁。
package demos

import "sync/atomic"

// Config 是一份不可变快照。拿到它的人只读，永不修改其内容。
type Config struct {
	data map[string]string
}

// Get 读取一个键。注意 Config 一旦发布就不可变，所以这里无需任何同步。
func (c *Config) Get(key string) (string, bool) {
	v, ok := c.data[key]
	return v, ok
}

// Manager 持有"当前生效"的配置指针，并发安全地支持热替换。
type Manager struct {
	v atomic.Pointer[Config]
}

// NewManager 用初始配置建一个 Manager。
func NewManager(initial map[string]string) *Manager {
	m := &Manager{}
	m.Reload(initial)
	return m
}

// Current 取当前配置快照。这是读热路径：一次原子 Load，无锁、多核可并行、零争用。
// atomic.Load 同时建立 happens-before，保证读者看到的是构造完整的 Config。
func (m *Manager) Current() *Config {
	return m.v.Load()
}

// Reload 整体替换配置。
// 关键纪律：拷贝一份入参再封装，确保发布出去的快照不会被调用方事后改动；
// 旧快照仍被在途读者持有，等它们读完、GC 自动回收。
//
// 真实接线：把这个方法挂到 fsnotify 文件监听回调、或配置中心的 watch 回调上即可。
func (m *Manager) Reload(newData map[string]string) {
	cp := make(map[string]string, len(newData))
	for k, v := range newData {
		cp[k] = v
	}
	m.v.Store(&Config{data: cp})
}
