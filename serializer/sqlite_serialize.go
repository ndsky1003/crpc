package serializer

import "github.com/ndsky1003/crpc/header"

type sqlite_serializer struct {
}

func NewSqliteSerialize() *sqlite_serializer {
	return new(sqlite_serializer)
}

func (this *sqlite_serializer) Serialize(h *header.Header, body []byte) error {
	return nil
}

func (this *sqlite_serializer) Deserialize() (h *header.Header, body []byte, err error) {
	return nil, nil, nil
}

var DefaultSqliteSerializer Serializer = NewSqliteSerialize()
