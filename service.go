package crpc

import (
	"fmt"
	"sync"
	"time"

	"github.com/ndsky1003/crpc/v2/codec"
	"github.com/ndsky1003/crpc/v2/coder"
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
	opt         *option_server
	sync.Mutex  //读是单线程，写加锁
}

func newService(server *server, codec codec.Codec, opt *option_server) *service {
	s := &service{
		server: server,
		codec:  codec,
		done:   make(chan struct{}),
		opt:    opt,
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
	if _, err = this.codec.ReadMetaData(h); err != nil {
		h.Release()
		this.close(false)
		logrus.Errorf("read meta data err:%v", err)
		return
	}

	bodyData, err := this.codec.ReadBodyData(h)
	if err != nil {
		h.Release()
		this.close(false)
		logrus.Errorf("read body data err:%v", err)
		return
	}
	var req verify_req
	if err = coder.Unmarshal(h.ReqCoderT, bodyData, &req); err != nil {
		h.Release()
		this.close(false)
		logrus.Errorf("first frame body is error:%v", err)
		return
	}

	if s := this.server.opt.Secret; s != nil {
		if req.Secret != *s {
			h.Release()
			this.close(false)
			logrus.Errorf("secret is invalid,please change")
			return
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
	h.Type = headertype.Res_Success
	if err = this.codec.WriteFrame(h, nil, verify_res{Success: true}); err != nil {
		h.Release()
		logrus.Errorf("write verify res is err :%v", err)
		this.close(true)
		return
	}
	h.Release()
	this.Unlock()
	h = nil
	for {
		//NOTE: client的心跳是15s,且有skip_heart的机制,最少应该2个间隔30秒,加上网络波动+5 = 35
		readdeadline := *this.opt.ReadDeadline
		this.codec.SetReadDeadline(time.Now().Add(time.Second * readdeadline))
		if h, err = this.codec.ReadHeader(); err != nil {
			err = fmt.Errorf("1%w,%v", ServerError, err)
			break
		}

		var metaData, bodyData []byte
		if /*h.Type&headertype.Res == 0*/ h.Type.IsReq() {
			this.codec.SetReadDeadline(time.Now().Add(time.Second * readdeadline))
			if metaData, err = this.codec.ReadMetaRawData(h); err != nil {
				err = fmt.Errorf("2%w,%v", ServerError, err)
				break
			}
		}

		this.codec.SetReadDeadline(time.Now().Add(time.Second * readdeadline))
		if bodyData, err = this.codec.ReadBodyRawData(h); err != nil {
			err = fmt.Errorf("3%w,%v", ServerError, err)
			break
		}

		switch h.Type {
		case headertype.Ping:
			h.Type = headertype.Pong
			go this.WriteFrame(h, nil, nil)
		case headertype.Req, headertype.Chunks, headertype.Msg: //forward
			if e := this.server.WriteRawData(h.ToService, h, metaData, bodyData); e != nil {
				e = fmt.Errorf("%w,%v", ServerError, e)
				logrus.Error(e)
				h.Type = headertype.Res_Err_Standard
				go this.WriteFrame(h, nil, e)
			}
		case headertype.Res_Success, headertype.Res_Err_Custom, headertype.Res_Err_Standard: //back forward
			if e := this.server.WriteRawData(h.FromService, h, metaData, bodyData); e != nil {
				logrus.Error(e)
			}
		default: //pong
		}
	}

	if h != nil && h.Type.IsReq() { //req
		h.Type = headertype.Res_Err_Standard
		if err := this.WriteFrame(h, nil, err); err != nil {
			err = fmt.Errorf("%w,%v", ServerError, err)
			logrus.Error(err)
		}
	}
	this.Close(true)
	logrus.Errorf("service:%s is die,err:%v\n", this.name, err)
}

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
	writedeadline := *this.opt.WriteDeadline
	if this.codec != nil {
		this.codec.SetWriteDeadline(time.Now().Add(time.Second * writedeadline))
		return this.codec.WriteFrameRawData(h, meta_data, data)
	}
	return nil
}

func (this *service) WriteFrame(h *header.Header, meta, v any) error {
	defer h.Release()
	this.Lock()
	defer this.Unlock()
	writedeadline := *this.opt.WriteDeadline
	if this.codec != nil {
		this.codec.SetWriteDeadline(time.Now().Add(time.Second * writedeadline))
		return this.codec.WriteFrame(h, meta, v)
	}
	return nil
}
