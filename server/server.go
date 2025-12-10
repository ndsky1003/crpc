package server

import (
	"context"
	"time"

	"github.com/ndsky1003/crpc/v3/comm/ut"
	"github.com/ndsky1003/net/server"
	"github.com/panjf2000/ants/v2"
)

type Server struct {
	*server.Server
}

func New(ctx context.Context, opts ...*Option) *Server {
	opt := Options().
		SetSecret(ut.GetEnv("CRPC_SECRET", "8620506fd4781174ec05fcacf816a12e")).
		SetGroupReplicas(ut.GetEnvInt("GROUP_REPLICAS", 100)).
		SetSendTimeout(30 * time.Second).
		SetBroadcastCounterExpiration(10 * time.Second).
		SetWorkerSize(300).
		Merge(opts...)
	s := &Server{}
	workPool, err := ants.NewPool(*opt.WorkerSize, ants.WithNonblocking(true))
	if err != nil {
		panic(err)
	}
	mgr := &server_mgr{
		opt:              &opt,
		broadcastCounter: NewBroadcastCounterAll(*opt.BroadcastCounterExpiration),
		workPool:         workPool,
	}
	s.Server = server.New(ctx, mgr, &opt.Option)
	return s
}

func (s *Server) Close() error {
	return s.Server.Close()
}
