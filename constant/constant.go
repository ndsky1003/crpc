package constant

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"reflect"
)

var (
	e1             = errors.New("")       //*errors.errorString
	e2             = fmt.Errorf("%w", e1) //*fmt.wrapError
	StandardError1 = reflect.TypeOf(e1)
	StandardError2 = reflect.TypeOf(e2)
)

var StandardErrors = []reflect.Type{
	StandardError1,
	StandardError2,
	reflect.TypeFor[*fs.PathError](),
	reflect.TypeFor[net.UnknownNetworkError](),
	reflect.TypeFor[net.InvalidAddrError](),
	reflect.TypeFor[*net.ParseError](),
}
