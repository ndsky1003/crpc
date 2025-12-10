package buffer

import (
	"bytes"
	"sync"
	"sync/atomic"
)

// 配置参数
const (
	calibrateCalls = 1000
	maxPercentile  = 2.0
	minValidSize   = 1024
)

// DynamicBufferPool 自动伸缩的 Buffer 池
type DynamicBufferPool struct {
	pool sync.Pool

	calls        uint64
	maxUsage     uint64
	calibratedSz uint64
}

// NewDynamicBufferPool 创建一个自动伸缩池
func NewDynamicBufferPool() *DynamicBufferPool {
	p := &DynamicBufferPool{
		calibratedSz: 4096,
	}

	p.pool.New = func() any {
		size := atomic.LoadUint64(&p.calibratedSz)
		return bytes.NewBuffer(make([]byte, 0, size))
	}

	return p
}

// Get 获取一个 Buffer
func (p *DynamicBufferPool) Get() *Buffer {
	b := p.pool.Get().(*bytes.Buffer)
	b.Reset()
	// 【关键修改】：注入 p (池子本身) 到 Buffer 中
	return &Buffer{Buffer: b, pool: p}
}

// Release 归还 Buffer
func (p *DynamicBufferPool) Release(b *Buffer) {
	if b == nil || b.Buffer == nil {
		return
	}

	used := uint64(b.Len())
	cap := uint64(b.Cap())

	// 采样与校准逻辑 (保持不变)
	for {
		oldMax := atomic.LoadUint64(&p.maxUsage)
		if used <= oldMax {
			break
		}
		if atomic.CompareAndSwapUint64(&p.maxUsage, oldMax, used) {
			break
		}
	}

	if atomic.AddUint64(&p.calls, 1) > calibrateCalls {
		if atomic.CompareAndSwapUint64(&p.calls, calibrateCalls+1, 0) {
			newMax := atomic.LoadUint64(&p.maxUsage)
			newMax = max(newMax, minValidSize)
			atomic.StoreUint64(&p.calibratedSz, newMax)
			atomic.StoreUint64(&p.maxUsage, 0)
		}
	}

	targetSz := atomic.LoadUint64(&p.calibratedSz)

	if cap > targetSz*uint64(maxPercentile) {
		b.Buffer = nil
		return
	}

	b.Reset()
	p.pool.Put(b.Buffer)

	b.Buffer = nil
	b.pool = nil
}

// Buffer 包装器
type Buffer struct {
	*bytes.Buffer
	pool *DynamicBufferPool
}

func (b *Buffer) Release() {
	if b == nil || b.pool == nil {
		return
	}
	b.pool.Release(b)
}

var defaultPool = NewDynamicBufferPool()

func Get() *Buffer {
	return defaultPool.Get()
}
