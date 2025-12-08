// 这里是伪代码,用于代码生成示例
package client

import (
	"errors"
	"sync"

	"github.com/ndsky1003/crpc/v3/coder"
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
func (c *msg_game) HandleMsg(method string, metaCoderT coder.T, reqCoderT coder.T, meta, body []byte, wg *sync.WaitGroup) (any, error) {
	switch method {
	case "PlayerInfo":
		req := &PlayerInfoReq{}
		err := coder.Unmarshal(reqCoderT, body, req)
		if wg != nil {
			wg.Done()
		}
		if err != nil {
			return nil, err
		}
		return c.PlayerInfo(req)
	default:
		if wg != nil {
			wg.Done()
		}
		return nil, errors.New("unknown method")
	}
}
