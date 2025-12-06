package client

import (
	"reflect"
	"sync"

	"github.com/ndsky1003/crpc/v3/protocol/header"
)

type client_handler interface {
	HandleMsg(header *header.Header, meta, body []byte, wg *sync.WaitGroup) (any, error)
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
