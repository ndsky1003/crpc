package client

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/compressor"
	"github.com/ndsky1003/crpc/v3/protocol"
)

const defaultChunkSize = 1 * 1024 * 1024 // 默认分片大小 1MB

// SendFile 发送文件
// server: 目标服务名
// method: 目标方法名 (e.g. "FileSvc.Upload")
// filePath: 本地文件路径
// opts: 额外选项
func (c *Client) SendFile(ctx context.Context, server, method, filePath string, opts ...*Option) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return err
	}
	if stat.IsDir() {
		return errors.New("path is a directory")
	}

	// 强制使用 Msgp 编码和 Raw 压缩（文件通常自带压缩或由上层处理，避免重复压缩带来的CPU消耗）
	// 设置较长的超时时间，防止大文件传输中断
	opt := c.opt.
		SetReqCoderT(coder.Msgp).
		SetCompressT(compressor.Raw).
		Merge(opts...)

	// 如果没有设置超时，给一个默认的大超时
	// 注意：V3 的 Call 内部通常有默认超时，这里可能需要覆盖
	// 实际代码中建议根据文件大小动态计算或由用户传入 Context 控制

	chunkSize := defaultChunkSize
	buf := make([]byte, chunkSize)

	// 发送的文件名使用 Base 名字，避免带上绝对路径
	destFileName := filepath.Base(filePath)

	var offset int64 = 0

	for {
		n, readErr := f.Read(buf)
		if n > 0 {
			// 构造请求体
			req := &protocol.FileTransfer{
				FileName: destFileName,
				Data:     buf[:n], // 注意：切片共享底层数组，Marshal 时如果不是立即序列化可能会有问题，但在同步 Call 中是安全的
				Offset:   offset,
				IsFinish: false,
			}

			// 如果读到了 EOF，且这是最后一块，可以直接标记 Finish
			// 但更安全的做法是读不到数据时再发一个空包或者利用这一次
			// 这里采用：读多少发多少，下一轮读不到再退出或发结束包

			// 发送分片
			// 这里的 reply 设为 nil，表示不关心具体的返回值内容，只关心 error
			if err := c.Call(ctx, server, method, req, nil, &opt); err != nil {
				return err
			}

			offset += int64(n)
		}

		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return readErr
		}
	}

	// 发送结束标记 (可以发一个空数据的包，标记 IsFinish=true)
	// 这样服务端可以进行收尾工作（如重命名临时文件、校验 hash 等）
	endReq := &protocol.FileTransfer{
		FileName: destFileName,
		Data:     nil,
		Offset:   offset,
		IsFinish: true,
	}
	if err := c.Call(ctx, server, method, endReq, nil, &opt); err != nil {
		return err
	}

	return nil
}
