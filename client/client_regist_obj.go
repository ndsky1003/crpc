package client

import (
	"context"
	"fmt"
	"reflect"

	"github.com/ndsky1003/crpc/v3/coder"
)


type client_handler interface {
	// HandleMsg(ctx context.Context, method string, metaCoderT coder.T, reqCoderT coder.T, meta, body []byte) (any, error)
	// meta 和 body 的类型从 []byte 改为 any
	// 这样既可以传 []byte (远程)，也可以传 struct 指针 (本地)
	HandleMsg(ctx context.Context, method string, metaCoderT coder.T, reqCoderT coder.T, meta, body any) (any, error)
}

func (this *Client) Register(rcvr any) error {
	if err := this.validateReceiver(rcvr); err != nil {
		return err
	}
	return this.register(rcvr, "", false)
}

func (this *Client) RegisterName(name string, rcvr any) error {
	if err := this.validateReceiverName(name, rcvr); err != nil {
		return err
	}
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

// validateReceiver 验证接收者的基本有效性
func (this *Client) validateReceiver(rcvr any) error {
	if rcvr == nil {
		return fmt.Errorf("crpc: receiver cannot be nil")
	}

	rcvrVal := reflect.ValueOf(rcvr)
	rcvrType := reflect.Indirect(rcvrVal).Type()

	if rcvrType.Kind() != reflect.Struct {
		return fmt.Errorf("crpc: receiver must be a struct or pointer to struct")
	}

	return nil
}

// validateReceiverName 验证服务名称和接收者的有效性
func (this *Client) validateReceiverName(name string, rcvr any) error {
	if name == "" {
		return fmt.Errorf("crpc: service name cannot be empty")
	}

	return this.validateReceiver(rcvr)
}
