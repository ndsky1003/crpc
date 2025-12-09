package ut

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ndsky1003/crpc/v3/protocol"
)

func ResolveOption[T any](old **T, new *T) {
	if new != nil {
		*old = new
	}

}

var (
	// 临时文件存放目录
	TempDir = ".tmp_uploads"
)

func init() {
	_ = os.MkdirAll(TempDir, 0755)
}

// WriteFile 服务端处理文件的帮助函数
// req: 客户端传来的 DTO
// saveDir: 文件最终保存的目录
func WriteFile(req *protocol.FileTransfer, saveDir string) error {
	if req.FileName == "" {
		return errors.New("filename is empty")
	}

	// 构造临时文件路径
	// 建议加上 SessionID 或 UUID 防止文件名冲突，这里简单演示使用文件名
	tmpPath := filepath.Join(TempDir, req.FileName+".tmp")

	// 如果是结束包
	if req.IsFinish {
		// 确保目标目录存在
		if err := os.MkdirAll(saveDir, 0755); err != nil {
			return err
		}
		destPath := filepath.Join(saveDir, req.FileName)

		// 移动/重命名临时文件到目标位置
		// Windows下如果目标存在会失败，Linux下会覆盖
		if err := os.Rename(tmpPath, destPath); err != nil {
			return fmt.Errorf("finalize file error: %v", err)
		}
		return nil
	}

	// 写入分片数据
	// 使用 O_CREATE | O_WRONLY，不使用 O_APPEND，而是利用 Seek (Offset) 确保并发或乱序（如果改用 UDP）安全
	// 但在 TCP RPC 顺序调用下，O_APPEND 也是可以的。为了严谨使用 Offset。
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// 写入指定偏移量
	if _, err := f.WriteAt(req.Data, req.Offset); err != nil {
		return err
	}

	return nil
}
