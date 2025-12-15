package buffer

import (
	"bytes"

	"github.com/ndsky1003/buffer/v2"
)

var globalPool = buffer.NewBufferPool(
	buffer.Options().
		SetMinSize(512).          // 最小 512B
		SetMaxSize(64 << 20).     // 最大 64MB
		SetMaxPercent(1.5).       // 推荐：1.5 倍冗余 (比之前的 2.0 更激进一点，利于回收)
		SetCalibratePeriod(1000), // 每 1000 次调用校准一次
)

// Buffer 包装器
type Buffer struct {
	*bytes.Buffer
}

func (b *Buffer) Release() {
	if b != nil && b.Buffer != nil {
		// 将底层 buffer 归还给新库
		globalPool.Put(b.Buffer)
		// 避免悬垂指针
		b.Buffer = nil
	}
}

func Get() *Buffer {
	return &Buffer{
		Buffer: globalPool.Get(),
	}
}
