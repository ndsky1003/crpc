package client

import (
	"context"
	"fmt"
	"reflect"

	"github.com/ndsky1003/crpc/v3/coder"
)

type client_handler interface {
	HandleMsg(ctx context.Context, method string, metaCoderT coder.T, reqCoderT coder.T, meta, body []byte) (any, error)
}

func (this *Client) Register(rcvr any) error {
	return this.register(rcvr, "", false)
}

func (this *Client) RegisterName(name string, rcvr any) error {
	return this.register(rcvr, name, true)
}

func (this *Client) register(rcvr any, name string, useName bool) error {
	rcvrVal := reflect.ValueOf(rcvr)
	rcvrType := reflect.Indirect(rcvrVal).Type()

	serviceName := name
	if !useName {
		serviceName = rcvrType.Name()
		if serviceName == "" {
			return fmt.Errorf("crpc: could not determine service name for %T", rcvr)
		}
	}

	if handler, ok := rcvr.(client_handler); ok {
		this.serviceMap.Store(serviceName, handler)
		return nil
	}

	return this.registerStructMethods(serviceName, rcvrVal)
}
