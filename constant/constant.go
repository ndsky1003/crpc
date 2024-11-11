package constant

import (
	"errors"
	"fmt"
	"reflect"
)

var (
	e1             = errors.New("")       //*errors.errorString
	e2             = fmt.Errorf("%w", e1) //*fmt.wrapError
	StandardError1 = reflect.TypeOf(e1)
	StandardError2 = reflect.TypeOf(e2)
)
