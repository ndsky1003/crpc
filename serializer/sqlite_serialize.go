package serializer

type sqlite_serialize struct {
}

func NewSqliteSerialize() *sqlite_serialize {
	return new(sqlite_serialize)
}

func (this *sqlite_serialize) Serialize(data []byte) error {
	return nil
}

func (this *sqlite_serialize) Deserialize() ([]byte, error) {
	return nil, nil
}
