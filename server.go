package crpc

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/ndsky1003/event/codec"
	"github.com/ndsky1003/event/msg"
	"github.com/ndsky1003/event/options"
	"github.com/sirupsen/logrus"
)

type server struct {
	codecFunc options.CreateServerCodecFunc
	mutex     sync.RWMutex
	seq       uint64
	monitor   map[*msg.EventTopic]map[uint64]struct{}
	services  map[uint64]*module

	reqTimeOut time.Duration //请求超时
	reqSeq     uint64
	reqMetas   map[uint64]*serverReqMeta
}

type serverReqMeta struct {
	reqServerID    uint64
	serverReqSeq   uint64
	serverReqCount uint64
	existErr       bool
	errs           []string
	Time           time.Time
	//clien info
	clientSeq uint64
	msg.EventType
}

func NewServer(opts ...*options.ServerOptions) *server {
	c := &server{
		services:   map[uint64]*module{},
		monitor:    map[*msg.EventTopic]map[uint64]struct{}{},
		reqTimeOut: 10,
		reqMetas:   map[uint64]*serverReqMeta{},
	}

	opt := options.Server().SetCodecFunc(func(conn io.ReadWriteCloser) (codec.Codec, error) {
		return codec.NewGobCodec(conn), nil
	}).Merge(opts...)
	if opt.CodecFunc != nil {
		c.codecFunc = *opt.CodecFunc
	}
	if opt.ReqTimeout != nil {
		c.reqTimeOut = *opt.ReqTimeout
	}
	go c.checkTimeOut()
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
		codec, err := this.codecFunc(conn)
		if err != nil {
			logrus.Error(err)
			continue
		}
		this.mutex.Lock()
		seq := this.seq
		seq = incSeqID(seq)
		this.seq = seq
		service := newService(this, seq, codec)
		this.services[this.seq] = service
		this.mutex.Unlock()
		go service.serve()
	}
}
