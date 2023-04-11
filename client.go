package crpc

import (
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/ndsky1003/event/codec"
	"github.com/ndsky1003/event/msg"
	"github.com/ndsky1003/event/options"
	"github.com/sirupsen/logrus"
)

// service - module -> func
type Client struct {
	name          string
	url           string
	moduleMap     sync.Map // map[string]*module
	l             sync.Mutex
	seq           uint64
	pending       map[uint64]*Call
	checkInterval time.Duration //链接检测
	heartInterval time.Duration //心跳间隔
	isStopHeart   bool          //是否关闭心跳
	connecting    bool          // client is connecting
}

func Dial(name, url string, opts ...*options.ClientOptions) *Client {
	c := &Client{
		name:          name,
		url:           url,
		pending:       make(map[uint64]*Call),
		checkInterval: 1,
		heartInterval: 5,
	}
	//合并属性
	opt := options.Client().SetCodecFunc(func(conn io.ReadWriteCloser) (codec.Codec, error) {
		return codec.NewGobCodec(conn), nil
	}).Merge(opts...)

	//属性设置开始
	if opt.Name != nil {
		c.name = *opt.Name
	}
	if opt.CodecFunc != nil {
		c.codecFunc = *opt.CodecFunc
	}

	if opt.CheckInterval != nil {
		c.checkInterval = *opt.CheckInterval
	}

	if opt.HeartInterval != nil {
		c.heartInterval = *opt.HeartInterval
	}

	if opt.IsStopHeart != nil {
		c.isStopHeart = *opt.IsStopHeart
	}
	go c.keepAlive()
	return c
}
func (this *Client) getConnecting() bool {
	this.mutex.Lock()
	defer this.mutex.Unlock()
	return this.connecting
}

func (this *Client) keepAlive() {
	for {
		if !this.getConnecting() {
			conn, err := net.Dial("tcp", this.url)

			if err != nil {
				logrus.Errorf("dail err:%v\n", err)
				time.Sleep(this.checkInterval * time.Second)
				continue
			}
			codec, err := this.codecFunc(conn)
			if err != nil {
				logrus.Errorf("codec err:%v\n", err)
				time.Sleep(this.checkInterval * time.Second)
				continue
			} else {
				if err := this.serve(codec); err != nil {
					logrus.Error("server:", err)
				}
				continue
			}
		} else { //heart
			if !this.isStopHeart {
				if call := this.emit_async(msg.MsgType_ping, ""); call != nil {
					err := call.Error
					if err != nil { //这里是同步触发的错误
						logrus.Error(err)
						if errors.Is(err, io.ErrShortWrite) || errors.Is(err, errLocalWrite) {
							this.stop(err)
						}
					}
				}
				time.Sleep(this.heartInterval * time.Second)
			} else {
				time.Sleep(this.checkInterval * time.Second) //下次去尝试连接
			}
		}
	}
}

func (this *Client) serve(codec codec.Codec) (err error) {
	this.mutex.Lock()
	defer func() {
		if err != nil {
			this.mutex.Unlock()
		}
	}()
	if err = codec.Write(&msg.Msg{T: msg.MsgType_varify, Name: this.name}); err != nil {
		return
	}
	var readFirstMsg msg.Msg
	if err = codec.Read(&readFirstMsg); err != nil {
		return
	}
	if readFirstMsg.T == msg.MsgType_prepared {
		//重连挂载已经有的event
		for tp := range this.events {
			if err = codec.Write(&msg.Msg{T: msg.MsgType_on, EventType: tp.GetEventType()}); err != nil {
				return
			}
		}
	}
	this.connecting = true
	this.codec = codec
	this.mutex.Unlock()
	go this.input(codec)
	return
}

func (this *Client) stop(err error) {
	this.mutex.Lock()
	defer this.mutex.Unlock()
	for _, call := range this.pending {
		call.Error = err
		logrus.Errorf("%+v,err:%v", call.Msg, call.Error)
		call.Do()
	}
	if this.connecting {
		this.codec.Close()
		this.codec = nil
	}
	this.seq = 0
	this.pending = make(map[uint64]*msg.Call)
	this.connecting = false
}

func (this *Client) StopHeart() {
	this.mutex.Lock()
	defer this.mutex.Unlock()
	this.isStopHeart = true

}

func (this *Client) PrintCall() {
	for index, msg := range this.pending {
		logrus.Infof("index:%d,msg:%+v\n", index, msg.Error)
	}
}

func (this *Client) input(codec codec.Codec) {
	var err error
	for err == nil {
		var gotMsg msg.Msg
		err = codec.Read(&gotMsg)
		if err != nil {
			err = errors.New("reading error body1: " + err.Error())
			break
		}
		switch gotMsg.T {
		case msg.MsgType_ping:
		case msg.MsgType_req:
			//fmt.Printf("client receive:%+v\n", gotMsg)
			go this.call(codec, &gotMsg)
		case msg.MsgType_res, msg.MsgType_on, msg.MsgType_pong:
			if gotMsg.T != msg.MsgType_pong {
				//fmt.Printf("client receive:%+v\n", gotMsg)
			}
			seq := gotMsg.ClientSeq
			this.mutex.Lock()
			call := this.pending[seq]
			delete(this.pending, seq)
			this.mutex.Unlock()
			if call != nil {
				if gotMsg.Error != "" {
					call.Error = ServerError(gotMsg.Error)
				}
				call.Do()
			}
		}
	}
	logrus.Errorf("read err:%+v", err)
	this.stop(err)
}
