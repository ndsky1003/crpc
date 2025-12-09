package server

import (
	"context"

	"github.com/ndsky1003/net/server"
)

type Server struct {
	*server.Server
}

func New(ctx context.Context, opts ...*Option) *Server {
	opt := Options().
		SetSecret("8620506fd4781174ec05fcacf816a12e").
		SetGroupReplicas(100).
		Merge(opts...)
	s := &Server{}
	mgr := &server_mgr{
		opt: &opt,
	}
	s.Server = server.New(ctx, mgr, &opt.Option)
	return s
}

func (s *Server) Close() error {
	return s.Server.Close()
}
