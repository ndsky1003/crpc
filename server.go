package crpc

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/ndsky1003/crpc/v2/codec"
	"github.com/ndsky1003/crpc/v2/header"
	"github.com/sirupsen/logrus"
)

type server struct {
	l            sync.Mutex
	index        uint32
	services     map[string]*service_mgr
	opt          *option_server
	codecGenFunc codecFunc
}

func NewServer(opts ...*option_server) *server {
	c := &server{
		services: map[string]*service_mgr{},
	}
	c.opt = OptionServer().SetReadDeadline(35).SetWriteDeadline(15).Merge(opts...)
	c.codecGenFunc = func(conn net.Conn) (codec.Codec, error) {
		return codec.NewCodec(conn), nil
	}
	return c
}

// addrs ["192.168.0.1:8080","192.168.0.2:8080"]
func (this *server) Listens(addrs []string) {
	for i := len(addrs) - 1; i >= 0; i-- {
		addr := addrs[i]
		// listenAddr := fmt.Sprintf("%v:%v", addr, port)
		if i != 0 {
			go this.listen(addr)
		} else {
			this.listen(addr)
		}
	}
}

//url:port
func (this *server) Listen(url string) {
	this.listen(url)
}

func (this *server) listen(url string) {
	if this == nil {
		panic("crpc server is nil")
	}
	listen, err := net.Listen("tcp", url)
	if err != nil {
		panic(fmt.Errorf("crpc server listen err:%w", err))
	}
	for {
		conn, err := listen.Accept()
		if err != nil {
			continue
		}
		codec, err := this.codecGenFunc(conn)
		if err != nil {
			conn.Close()
			logrus.Error(err)
			continue
		}
		id := atomic.AddUint32(&this.index, 1)
		service := newService(this, id, codec, this.opt)
		go service.serve()
	}
}

func (this *server) getService(name string) (*service, error) {
	if name == "" {
		return nil, errors.New("service name is empty")
	}
	this.l.Lock()
	sg, ok := this.services[name]
	this.l.Unlock()
	if ok {
		return sg.RandOne(), nil
	} else {
		return nil, fmt.Errorf("service name:%s not exist", name)
	}
}

func (this *server) getServiceBySid(name string, sid uint32) (*service, error) {
	this.l.Lock()
	sg, ok := this.services[name]
	this.l.Unlock()
	if ok {
		if s := sg.GetService(sid); s != nil {
			return s, nil
		} else {
			return nil, fmt.Errorf("service name:%s sid:%d not exist", name, sid)
		}
	} else {
		return nil, fmt.Errorf("service name:%s sid:%d not exist", name, sid)
	}
}

func (this *server) addService(s *service) error {
	if s.name == "" {
		return errors.New("service name is empty")
	}
	this.l.Lock()
	defer this.l.Unlock()
	sg, ok := this.services[s.name]
	if ok {
		return sg.addService(s)
	} else {
		sg = new_service_mgr()
		if err := sg.addService(s); err != nil {
			return err
		}
		this.services[s.name] = sg
	}
	return nil
}

func (this *server) removeService(s *service, isClose bool) error {
	if s.name == "" {
		return errors.New("service name is empty")
	}
	this.l.Lock()
	defer this.l.Unlock()
	sg, ok := this.services[s.name]
	if ok {
		if w, err := sg.removeService(s, isClose); err == nil && w == 0 {
			delete(this.services, s.name)
		}
	}
	return nil
}

func (this *server) WriteRawData(name string, h *header.Header, meta_data, data []byte) error {
	s, err := this.getService(name)
	if err != nil {
		return err
	}
	if s == nil {
		return fmt.Errorf("%v,暂无可用的service", name)
	}
	go s.WriteRawData(h, meta_data, data)
	return nil
}

func (this *server) WriteRawDataBySid(fromserver string, sid uint32, h *header.Header, meta_data, data []byte) error {
	s, err := this.getServiceBySid(fromserver, sid)
	if err != nil {
		return err
	}
	go s.WriteRawData(h, meta_data, data)
	return nil
}

func (this *server) WriteFrame(name string, h *header.Header, meta, body any) error {
	s, err := this.getService(name)
	if err != nil {
		return err
	}
	if s == nil {
		return fmt.Errorf("%v,暂无可用的service", name)
	}
	go s.WriteFrame(h, meta, body)
	return nil
}
