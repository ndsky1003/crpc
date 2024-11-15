package crpc

import (
	"reflect"

	"github.com/ndsky1003/crpc/v2/constant"
)

func RegistCustomError(rt reflect.Type) {
	constant.StandardErrors = append(constant.StandardErrors, rt)
}
