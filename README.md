# crpc
中心服务的rpc，采用注册机制


#### 支持的方法签名

```golang
// 这里是伪代码,用于代码生成示例
//
//go:generate gencrpcserverv3
//go:generate gencrpcclientv3 --out_dir=./db --package=db
package client

import (
	"context"
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

// @crpc:CallType:Call,Send,Go,Broadcast
// @crpc:Client: ccclient
// @crpc:Module: crpc
// @crpc:Service: db
// @crpc:FuncName:PlayerInfo1_gai
func (*msg_game) PlayerInfo1(meta *Meta, req *PlayerInfoReq) (*PlayerInfoRes, error) {
	return nil, nil
}

// @crpc:IsSkip:true
func (*msg_game) PlayerInfo2(req *PlayerInfoReq) (*PlayerInfoRes, error) {
	return nil, nil
}

func (*msg_game) PlayerInfo3(ctx context.Context, meta *Meta, req *PlayerInfoReq) error {
	return nil
}

func (*msg_game) PlayerInfo4(meta *Meta, req *PlayerInfoReq) error {
	return nil
}

func (*msg_game) PlayerInfo7(meta Meta, req *PlayerInfoReq) error {
	return nil
}

func (*msg_game) PlayerInfo5(req *PlayerInfoReq) error {
	return nil
}

func (*msg_game) PlayerInfo6(req PlayerInfoReq) error {
	return nil
}
```


