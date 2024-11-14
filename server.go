package crpc

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/ndsky1003/crpc/v2/codec"
	"github.com/ndsky1003/crpc/v2/header"
	"github.com/sirupsen/logrus"
)

type server struct {
	sync.RWMutex
	services     map[string]*service_mgr
	opt          *option_server
	codecGenFunc codecFunc
}

func NewServer(opts ...*option_server) *server {
	c := &server{
		services: map[string]*service_mgr{},
	}
	c.opt = OptionServer().Merge(opts...)
	c.codecGenFunc = func(conn io.ReadWriteCloser) (codec.Codec, error) {
		return codec.NewCodec(conn), nil
	}
	return c
}

// addrs ["192.168.0.1","192.168.0.2"]
// port 8080
func (this *server) Listens(addrs []string, port int) {
	for i := len(addrs) - 1; i >= 0; i-- {
		addr := addrs[i]
		listenAddr := fmt.Sprintf("%v:%v", addr, port)
		if i != 0 {
			go this.listen(listenAddr)
		} else {
			this.listen(listenAddr)
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
		service := newService(this, codec)
		go service.serve()
	}
}

func (this *server) getService(name string) (*service, error) {
	if name == "" {
		return nil, errors.New("service name is empty")
	}
	this.Lock()
	defer this.Unlock()
	if sg, ok := this.services[name]; ok {
		return sg.RandOne(), nil
	} else {
		return nil, fmt.Errorf("service name:%s not exist", name)
	}
}

func (this *server) addService(s *service) error {
	if s.name == "" {
		return errors.New("service name is empty")
	}
	this.Lock()
	defer this.Unlock()
	sg, ok := this.services[s.name]
	if ok {
		sg.addService(s)
	} else {
		sg = &service_mgr{}
		sg.addService(s)
		this.services[s.name] = sg
	}
	return nil
}

func (this *server) removeService(s *service, isClose bool) error {
	if s.name == "" {
		return errors.New("service name is empty")
	}
	this.Lock()
	defer this.Unlock()
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
