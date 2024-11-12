package crpc

import (
	"fmt"
	"sync"

	"github.com/ndsky1003/crpc/v2/codec"
	"github.com/ndsky1003/crpc/v2/header"
	"github.com/ndsky1003/crpc/v2/header/headertype"
	"github.com/sirupsen/logrus"
)

//-----------------------------service----------------------------

// 一个服务名字,可能有多个配套的service,根据权重来负载均衡,并
// 权重, >0 表示负载均衡,<=0表示互斥,并且保留负数,最小那个,-999 会强制踢掉-998的
type service struct {
	name        string
	done        chan struct{}
	fingerprint int
	weight      int
	server      *server
	codec       codec.Codec
	sync.Mutex  //读是单线程，写加锁
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
		this.close(false)
		logrus.Errorf("first frame header is error:%+v", err)
		return
	}
	if h.Type != headertype.Verify {
		h.Release()
		this.close(false)
		logrus.Error("first frame header is error")
		return
	}
	if err := this.codec.Drop(int(h.MetaLen)); err != nil {
		h.Release()
		this.close(false)
		logrus.Error("drop meta data err:%v", err)
		return
	}

	var req verify_req
	if err = this.codec.ReadBody(h, &req); err != nil {
		h.Release()
		logrus.Errorf("first frame body is error:%v", err)
		this.codec.Close()
		return
	}
	if s := this.server.opt.Secret; s != nil {
		if req.Secret != *s {
			h.Release()
			this.close(false)
			logrus.Errorf("secret  is invalid,please change")
		}
	}

	this.name = req.Name
	this.weight = req.Weight
	if err = this.server.addService(this); err != nil {
		h.Release()
		this.close(false)
		logrus.Errorf("add map is error:%v", err)
		return
	}
	this.Lock()
	if err = this.codec.WriteFrame(h, nil, verify_res{Success: true}); err != nil {
		h.Release()
		logrus.Errorf("write verify res is err :%v", err)
		this.close(true)
		return
	}
	h.Release()
	this.Unlock()
	for err == nil {
		h, e := this.codec.ReadHeader()
		if e != nil {
			err = e
			continue
		}

		var metaData []byte
		if h.Type&headertype.Res != 0 {
			metaData = make([]byte, h.MetaLen)
			if err = this.codec.Read(metaData); err != nil {
				err = fmt.Errorf("%w,%v", ServerError, err)
				continue
			}
		}

		bodyData := make([]byte, h.BodyLen)
		if err = this.codec.Read(bodyData); err != nil {
			err = fmt.Errorf("%w,%v", ServerError, err)
			continue
		}

		//logrus.Infof("data:%+v", data)
		switch h.Type {
		case headertype.Ping:
			h.Type = headertype.Pong
			go func() {
				defer h.Release()
				this.Write(h, nil)
			}()
		case headertype.Req, headertype.Chunks, headertype.Msg: //forward
			if e := this.server.WriteRawData(h.ToService, h, metaData, bodyData); e != nil {
				logrus.Error(e)
				h.Type = headertype.Res_Err_Standard
				go this.Write(h, e.Error())
			}
		case headertype.Res_Success, headertype.Res_Err_Custom, headertype.Res_Err_Standard: //back forward
			if e := this.server.WriteRawData(h.FromService, h, metaData, bodyData); e != nil {
				logrus.Error(e)
			}
		default: //pong
		}
	}
	this.Close(true)
	logrus.Errorf("service:%s is die,err:%v\n", this.name, err)
}

//	func (this *service) close() error {
//		this.server.removeService(this.name)
//		return this.codec.Close()
//	}

func (this *service) Close(removeFromMgr bool) error {
	this.Lock()
	defer this.Unlock()
	return this.close(removeFromMgr)
}

func (this *service) close(removeFromMgr bool) error {
	if removeFromMgr && this.server != nil {
		this.server.removeService(this, false)
		this.server = nil
	}
	if this.codec != nil {
		if err := this.codec.Close(); err != nil {
			return err
		}
		this.codec = nil
	}
	return nil
}

func (this *service) WriteRawData(h *header.Header, meta_data, data []byte) error {
	defer h.Release()
	this.Lock()
	defer this.Unlock()
	return this.codec.WriteFrameRawData(h, meta_data, data)
}
func (this *service) Write(h *header.Header, v any) error {
	this.Lock()
	defer this.Unlock()
	return this.codec.WriteFrame(h, nil, v)
}
