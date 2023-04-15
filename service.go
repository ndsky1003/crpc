package crpc

import (
	"sync"

	"github.com/ndsky1003/crpc/codec"
	"github.com/ndsky1003/crpc/header"
	"github.com/ndsky1003/crpc/header/headertype"
	"github.com/sirupsen/logrus"
)

//-----------------------------service----------------------------

type service struct {
	name   string
	done   chan struct{}
	server *server
	codec  codec.Codec
	mutex  sync.Mutex //读是单线程，写加锁
}

func newService(server *server, codec codec.Codec) *service {
	s := &service{
		server: server,
		codec:  codec,
		done:   make(chan struct{}),
	}
	return s
}

func (this *service) serve() {
	if this == nil {
		return
	}
	h, err := this.codec.ReadHeader()
	if err != nil {
		h.Release()
		this.codec.Close()
		logrus.Errorf("first frame header is error:%+v", err)
		return
	}
	if h.Type != headertype.Verify {
		h.Release()
		this.codec.Close()
		logrus.Error("first frame header is error")
		return
	}
	var req verify_req
	if err = this.codec.ReadBody(&req); err != nil {
		h.Release()
		logrus.Errorf("first frame body is error:%v", err)
		this.codec.Close()
		return
	}
	if req.Secret != this.server.Secret {
		h.Release()
		logrus.Errorf("verify is error")
		this.codec.Close()
		return
	}

	this.name = req.Name
	if err = this.server.addService(this.name, this); err != nil {
		logrus.Errorf("add map is error:%v", err)
		h.Release()
		this.codec.Close()
		return
	}
	this.mutex.Lock()
	if err = this.codec.Write(h, verify_res{Success: true}); err != nil {
		h.Release()
		logrus.Errorf("write verify res is err :%v", err)
		this.codec.Close()
		return
	}
	h.Release()
	this.mutex.Unlock()
	for err == nil {
		h, e := this.codec.ReadHeader()
		if e != nil {
			err = e
			continue
		}
		//logrus.Infof("header:%+v", h)
		var data []byte
		if err = this.codec.ReadBodyRawData(&data); err != nil {
			h.Release()
			continue
		}
		//logrus.Infof("data:%+v", data)
		switch h.Type {
		case headertype.Ping:
			h.Type = headertype.Pong
			go this.WriteRawData(h, data)
		case headertype.Req, headertype.Chunks, headertype.Msg: //forward
			if e := this.server.WriteRawData(h.ToService, h, data); e != nil {
				logrus.Error(e)
				h.Type = headertype.Reply_Error
				go this.Write(h, e.Error())
			}
		case headertype.Reply_Success, headertype.Reply_Error: //back forward
			if e := this.server.WriteRawData(h.FromService, h, data); e != nil {
				logrus.Error(e)
			}
		default: //pong
		}
	}
	this.close()
	logrus.Errorf("service:%s is die,err:%v\n", this.name, err)
}

func (this *service) close() error {
	this.server.removeService(this.name)
	return this.codec.Close()
}

func (this *service) WriteRawData(h *header.Header, data []byte) error {
	defer h.Release()
	this.mutex.Lock()
	defer this.mutex.Unlock()
	return this.codec.WriteRawData(h, data)
}
func (this *service) Write(h *header.Header, v any) error {
	defer h.Release()
	this.mutex.Lock()
	defer this.mutex.Unlock()
	return this.codec.Write(h, v)
}
