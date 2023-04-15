package crpc

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/ndsky1003/crpc/codec"
	"github.com/ndsky1003/crpc/header"
	"github.com/ndsky1003/crpc/options"
	"github.com/sirupsen/logrus"
)

type server struct {
	sync.RWMutex
	services     map[string]*service
	Secret       string
	codecGenFunc codecFunc
}

func NewServer(opts ...*options.ServerOptions) *server {
	c := &server{
		services: map[string]*service{},
	}
	opt := options.Server().Merge(opts...)
	//属性设置开始
	if opt.Secret != nil {
		c.Secret = *opt.Secret
	}
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
		panic("server is nil")
	}
	listen, err := net.Listen("tcp", url)
	if err != nil {
		panic(err)
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
	if s, ok := this.services[name]; ok {
		return s, nil
	} else {
		return nil, fmt.Errorf("service name:%s not exist", name)
	}
}

func (this *server) addService(name string, si *service) error {
	if name == "" {
		return errors.New("service name is empty")
	}
	this.Lock()
	defer this.Unlock()
	if _, ok := this.services[name]; ok {
		return fmt.Errorf("service name:%s exist", name)
	}
	this.services[name] = si
	logrus.Info("add service:", name)
	return nil
}

func (this *server) removeService(name string) error {
	if name == "" {
		return errors.New("service name is empty")
	}
	this.Lock()
	defer this.Unlock()
	delete(this.services, name)
	return nil
}

func (this *server) WriteRawData(name string, h *header.Header, data []byte) error {
	s, err := this.getService(name)
	if err != nil {
		return err
	}
	//if h.Type == headertype.Chunks {
	//logrus.Infof("forward:header:%+v,data:%+v\n", h, data)
	//}
	go s.WriteRawData(h, data)
	return nil
}
