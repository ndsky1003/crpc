package buffer

import (
	"bytes"
	"sync"
)

// 设定一个回收的阈值，防止曾经处理过大包的 buffer 长期占用内存
// 比如：如果 buffer 扩容超过了 64KB，归还时直接丢弃，不放回池子
const maxCacheCap = 64 * 1024 // 64KB

type Buffer struct {
	*bytes.Buffer
}

var pool = &sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 4*1024)) // 4KB 起步
	},
}

func Get() *Buffer {
	b := pool.Get().(*bytes.Buffer)
	b.Reset() // 必须重置，虽然 Release 时也重置了，但双重保险
	return &Buffer{Buffer: b}
}

func (b *Buffer) Release() {
	if b == nil || b.Buffer == nil {
		return
	}

	// 【关键策略】：判断容量
	// 如果容量太大（说明之前处理过突发大包），就不放回池子了，让 GC 回收掉
	// 这样可以避免池子里的对象由于一次突发流量全部变成几 MB 大小，撑爆内存
	if b.Cap() > maxCacheCap {
		b.Buffer = nil // 解除引用
		return
	}

	b.Reset() // 重置 len=0，保留 cap
	pool.Put(b.Buffer)
	b.Buffer = nil // 防止重复 Release 或 Release 后误用
}
