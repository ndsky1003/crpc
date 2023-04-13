package crpc

type verify_req struct {
	Name   string
	Secret string
}
type verify_res struct {
	Success bool
}
