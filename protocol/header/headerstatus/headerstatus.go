package headerstatus

type T uint8

const (
	StatusOK T = 1 << iota
	Failed
)
