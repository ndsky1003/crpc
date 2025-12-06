package client

import (
	"errors"
	"sync"

	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/protocol/header"
)

type msg_game struct{}

type PlayerInfoReq struct {
}
type PlayerInfoRes struct {
}

func (*msg_game) PlayerInfo(req *PlayerInfoReq) (*PlayerInfoRes, error) {
	return nil, nil
}

// code_gen
func (c *msg_game) HandleMsg(header *header.Header, meta, body []byte, wg *sync.WaitGroup) (any, error) {
	switch header.Method {
	case "PlayerInfo":
		req := &PlayerInfoReq{}
		coder.Unmarshal(header.ReqCoderT, body, req)
		wg.Done()
		return c.PlayerInfo(req)
	default:
		wg.Done()
		return nil, errors.New("unknown method")
	}
}
