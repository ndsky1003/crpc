package crpc

import (
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/sirupsen/logrus"
)

func init() {

	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,                  // 完整时间戳
		TimestampFormat: "2006-01-02 15:04:05", // 自定义时间戳格式
		ForceColors:     true,                  // 强制颜色输出
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			// 定制调用者信息
			fileLine := fmt.Sprintf("%s:%d", filepath.Base(f.File), f.Line)
			return "", fileLine
		},
	})
	logrus.SetReportCaller(true)
}
