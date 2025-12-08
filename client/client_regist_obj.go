package client

import (
	"context"
	"reflect"

	"github.com/ndsky1003/crpc/v3/coder"
)

type client_handler interface {
	//WARN: code_gen ,wg 可能为nil,用于等待释放meta与body buffer
	HandleMsg(ctx context.Context, method string, metaCoderT coder.T, reqCoderT coder.T, meta, body []byte) (any, error)
}

func (this *Client) Register(rcvr client_handler) error {
	return this.register(rcvr, "", false)
}

func (this *Client) RegisterName(name string, rcvr client_handler) error {
	return this.register(rcvr, name, true)
}

func (this *Client) register(rcvr any, name string, useName bool) error {
	rcvr_value := reflect.ValueOf(rcvr)
	sname := name
	if !useName {
		sname = reflect.Indirect(rcvr_value).Type().Name()
	}
	this.serviceMap.Store(sname, rcvr)
	return nil
}

func (this *Client) RegisterFunc(fn client_handler) error {
	rcvr_value := reflect.ValueOf(fn)
	sname := reflect.Indirect(rcvr_value).Type().Name()
	this.serviceMap.Store(sname, fn)
	return nil
}
