package crpc

type verify_req struct {
	Name   string
	Weight int
	Secret string
}
type verify_res struct {
	Success bool
}
