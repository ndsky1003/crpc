package comm

import "errors"

var UnmarshalError = errors.New("error occurred in Unmarshal")
var NotImplementProtoMessageError = errors.New("param must implement proto.Message")
