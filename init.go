package crpc

import "github.com/sirupsen/logrus"

func init() {
	logrus.SetReportCaller(true)
}
