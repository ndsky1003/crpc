package server

import (
	"context"
	"errors"
	"time"

	"github.com/ndsky1003/crpc/v3/comm/ut"
	"github.com/ndsky1003/net/server"
	"github.com/panjf2000/ants/v2"
)

type Server struct {
	*server.Server
	mgr *server_mgr
}

func New(ctx context.Context, opts ...*Option) (*Server, error) {
	opt := Options().
		SetSecret(ut.GetEnv("CRPC_SECRET", "")).
		SetGroupReplicas(ut.GetEnvInt("GROUP_REPLICAS", 100)).
		SetSendTimeout(30 * time.Second).
		SetBroadcastCounterExpiration(10 * time.Second).
		SetWorkerSize(5000).
		Merge(opts...)
	if *opt.Secret == "" {
		return nil, errors.New("CRPC_SECRET environment variable is required for security")
	}
	s := &Server{}
	workPool, err := ants.NewPool(*opt.WorkerSize, ants.WithNonblocking(true))
	if err != nil {
		return nil, err
	}
	mgr := &server_mgr{
		opt:              &opt,
		broadcastCounter: NewBroadcastCounterAll(*opt.BroadcastCounterExpiration),
		workPool:         workPool,
	}
	s.mgr = mgr
	s.Server = server.New(ctx, mgr, &opt.Option)
	return s, nil
}

// WARN: 这里面如果有异步，那么必须将Context Clone一份来维持新的生命周期，因为里面的Header是池化对象，有可能已经释放掉了
func (s *Server) Use(middleware ...HandlerFunc) {
	s.mgr.Use(middleware...)
}

func (s *Server) Close() error {
	if s.Server != nil {
		return s.Server.Close()
	}
	return nil
}
