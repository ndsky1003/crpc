// 这里是伪代码,用于代码生成示例
package client

import (
	"context"
	"errors"

	"github.com/ndsky1003/crpc/v3/coder"
)

type msg_game struct{}

type PlayerInfoReq struct {
}

type Meta struct {
}

type PlayerInfoRes struct {
}

func (*msg_game) PlayerInfo(ctx context.Context, meta *Meta, req *PlayerInfoReq) (*PlayerInfoRes, error) {
	return nil, nil
}

// code_gen
func (c *msg_game) HandleMsg(ctx context.Context, method string, metaCoderT coder.T, reqCoderT coder.T, meta, body []byte) (any, error) {
	switch method {
	case "PlayerInfo":
		req := &PlayerInfoReq{}
		meta := &Meta{}
		if err := coder.Unmarshal(reqCoderT, body, req); err != nil {
			return nil, err
		}
		if err := coder.Unmarshal(metaCoderT, body, meta); err != nil {
			return nil, err
		}
		return c.PlayerInfo(ctx, meta, req)
	default:
		return nil, errors.New("unknown method")
	}
}
