//go:generate msgp --tests=false
package protocol

type VerifyReq struct {
	Name   string
	Weight int
}
