package crpc

type verify_req struct {
	Name string
	Pwd  string
}
type verify_res struct {
	Success bool
}
