package crpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ndsky1003/crpc/codec"
	"github.com/ndsky1003/crpc/coder"
	"github.com/ndsky1003/crpc/compressor"
	"github.com/ndsky1003/crpc/header"
	"github.com/ndsky1003/crpc/header/headertype"
	"github.com/ndsky1003/crpc/options"
	"github.com/sirupsen/logrus"
)

type CodecFunc func(conn io.ReadWriteCloser) (codec.Codec, error)

// service - module -> func
type Client struct {
	name          string
	url           string
	moduleMap     sync.Map // map[string]*module
	coderType     coder.CoderType
	compressType  compressor.CompressType
	l             sync.Mutex
	codecGenFunc  CodecFunc
	codec         codec.Codec
	seq           uint64
	pending       map[uint64]*Call
	checkInterval time.Duration //链接检测
	heartInterval time.Duration //心跳间隔
	timeout       time.Duration // 负数 不失效
	isStopHeart   bool          //是否关闭心跳
	connecting    bool          // client is connecting
}

func Dial(name, url string, opts ...*options.ClientOptions) *Client {
	c := &Client{
		name:          name,
		url:           url,
		pending:       make(map[uint64]*Call),
		coderType:     coder.JSON,
		compressType:  compressor.Raw,
		checkInterval: 1,
		heartInterval: 5,
		timeout:       -1,
	}
	//合并属性
	opt := options.Client().Merge(opts...)
	//属性设置开始
	if opt.CoderType != nil {
		c.coderType = *opt.CoderType
	}
	if opt.CompressType != nil {
		c.compressType = *opt.CompressType
	}
	c.codecGenFunc = func(conn io.ReadWriteCloser) (codec.Codec, error) {
		return codec.NewCodec(conn, options.Codec().SetCoderType(c.coderType).SetCompressorType(c.compressType)), nil
	}
	if opt.Timeout != nil {
		c.timeout = *opt.Timeout
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
	this.l.Lock()
	defer this.l.Unlock()
	return this.connecting
}

func (this *Client) keepAlive() {
	for {
		if !this.getConnecting() {
			conn, err := net.Dial("tcp", this.url)

			if err != nil {
				log.Printf("dail err:%v\n", err)
				time.Sleep(this.checkInterval * time.Second)
				continue
			}
			codec, err := this.codecGenFunc(conn)
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
				h := header.Get()
				h.Type = headertype.Ping
				if err := this.send(h, nil); err != nil {
					logrus.Error(err)
					if errors.Is(err, io.ErrShortWrite) || errors.Is(err, WriteError) || errors.Is(err, codec.WriteError) {
						this.stop(err)
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
	this.l.Lock()
	defer func() {
		if err != nil {
			this.l.Unlock()
		}
	}()
	h := header.Get()
	h.Type = headertype.Verify
	if err = codec.Write(h, verify_req{Name: this.name, Pwd: "kasjfj"}); err != nil {
		return
	}
	header.Release(h)
	h, err = codec.ReadHeader()
	if err != nil {
		return err
	}
	if h.Type == headertype.Verify {
		err = fmt.Errorf("%w,headertype:%d is invalid", VerifyError, h.Type)
		return
	}
	var res verify_res
	if err = codec.ReadBody(&res); err != nil {
		return
	}
	header.Release(h)
	if !res.Success {
		err = fmt.Errorf("%w,verify failed", VerifyError)
		return
	}
	this.connecting = true
	this.codec = codec
	this.l.Unlock()
	go this.input(codec)
	return
}

func (this *Client) stop(err error) {
	this.l.Lock()
	defer this.l.Unlock()
	for _, call := range this.pending {
		call.Error = err
		call.done()
	}
	if this.connecting {
		this.codec.Close()
		this.codec = nil
	}
	this.seq = 0
	this.pending = make(map[uint64]*Call)
	this.connecting = false
}

func (this *Client) StopHeart() {
	this.l.Lock()
	defer this.l.Unlock()
	this.isStopHeart = true
}

func (this *Client) PrintCall() {
	for index, call := range this.pending {
		logrus.Infof("index:%d,msg:%+v\n", index, call.Error)
	}
}

func (this *Client) func_call(mod, method string, reqData []byte) (ret any, err error) {
	if v, ok := this.moduleMap.Load(mod); !ok {
		err = fmt.Errorf("%w,module:%s is not exist", FuncError, mod)
		return
	} else {
		mod := v.(module)
		if mtype, ok := mod.methods[method]; !ok {
			err = fmt.Errorf("%w,module:%s,method:%s is not exist", FuncError, mod, method)
			return
		} else {
			var argv, replyv reflect.Value
			argIsValue := false
			if mtype.ArgType.Kind() == reflect.Pointer {
				argv = reflect.New(mtype.ArgType.Elem())
			} else {
				argv = reflect.New(mtype.ArgType)
				argIsValue = true
			}
			if err = this.Unmarshal(&reqData, argv.Interface()); err != nil {
				return
			}
			if argIsValue {
				argv = argv.Elem()
			}
			replyv = reflect.New(mtype.ReplyType.Elem())
			switch mtype.ReplyType.Elem().Kind() {
			case reflect.Map:
				replyv.Elem().Set(reflect.MakeMap(mtype.ReplyType.Elem()))
			case reflect.Slice:
				replyv.Elem().Set(reflect.MakeSlice(mtype.ReplyType.Elem(), 0, 0))
			}
			function := mtype.method.Func
			returnValues := function.Call([]reflect.Value{mod.rcvr, argv, replyv})
			errInter := returnValues[0].Interface()
			if errInter != nil {
				err = errInter.(error)
			} else {
				ret = replyv.Interface()
			}
			return
		}
	}
}

func (this *Client) input(codec codec.Codec) {
	var err error
	for err == nil {
		h, err := this.codec.ReadHeader()
		if err != nil {
			err = fmt.Errorf("%w,%v", ReadError, err)
			break
		}
		switch h.Type {
		case headertype.Ping, headertype.Pong:
			if err = this.codec.ReadBodyData(nil); err != nil {
				err = fmt.Errorf("%w,%v", ReadError, err)
				break
			}
			if h.Type == headertype.Ping {
				go func() {
					defer header.Release(h)
					h.Type = headertype.Pong
					if e := this.send(h, nil); e != nil {
						log.Println(e)
					}
				}()
			} else {
				header.Release(h)
			}
		case headertype.Msg:
			var data []byte
			if err = this.codec.ReadBodyData(&data); err != nil {
				err = fmt.Errorf("%w,%v", ReadError, err)
				break
			}
			go func() {
				defer header.Release(h)
				if _, e := this.func_call(h.Module, h.Method, data); e != nil {
					logrus.Error(e)
				}
			}()
		case headertype.Req:
			var data []byte
			if err = this.codec.ReadBodyData(&data); err != nil {
				err = fmt.Errorf("%w,%v", ReadError, err)
				break
			}
			go func() {
				defer header.Release(h)
				var v any
				if ret, e := this.func_call(h.Module, h.Method, data); e != nil {
					h.Type = headertype.Reply_Error
					v = e
				} else {
					h.Type = headertype.Reply_Success
					v = ret
				}
				if e := this.send(h, v); e != nil {
					log.Println(e)
				}
			}()
		case headertype.Reply_Success, headertype.Reply_Error: //响应
			seq := h.Seq
			this.l.Lock()
			call := this.pending[seq]
			delete(this.pending, seq)
			this.l.Unlock()
			switch {
			case call == nil:
				err = this.codec.ReadBody(nil)
				if err != nil {
					err = errors.New("reading error body: " + err.Error())
				}
			case h.Type == headertype.Reply_Error:
				var errStr string
				if err := this.codec.ReadBody(&errStr); err != nil {
					err = errors.New("reading error body: " + err.Error())
					call.Error = fmt.Errorf("%w,err:%v", ServerError, err)
				} else {
					call.Error = fmt.Errorf("%w,err:%v", ServerError, errStr)
				}
				call.done()
			default:
				err = this.codec.ReadBody(call.Ret)
				if err != nil {
					call.Error = errors.New("reading body " + err.Error())
				}
				call.done()
			}
			header.Release(h)
		}
	}
	logrus.Error("read err:%+v\n", err)
	this.stop(err)
}

func (this *Client) parseMoudleFunc(moduleFunc string) (module, function string, err error) {
	if moduleFunc == "" {
		err = fmt.Errorf("%w,moduleFunc is empty", ModuleFuncError)
		return
	}
	modulefuncs := strings.Split(moduleFunc, ".")
	if len(module) != 2 {
		err = ModuleFuncError
		return
	}
	module, function = modulefuncs[0], modulefuncs[1]
	return

}

// 对外的方法 sync
func (this *Client) Call(ctx context.Context, server string, moduleFunc string, req, ret any) error {
	if this.timeout > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), this.timeout)
		defer cancel()
		call := this.Go(server, moduleFunc, req, ret, make(chan *Call, 1))
		select {
		case <-ctx.Done():
			return ReqTimeOutError
		case <-call.Done:
			return call.Error
		}
	} else {
		call := <-this.Go(server, moduleFunc, req, ret, make(chan *Call, 1)).Done
		return call.Error
	}
}

// async
func (this *Client) Go(server string, moduleFunc string, req, ret any, done chan *Call) *Call {
	call := new(Call)
	if done == nil {
		done = make(chan *Call, 10) // buffered.
	} else {
		if cap(done) == 0 {
			log.Panic("crpc: done channel is unbuffered")
		}
	}
	call.Done = done
	call.Req = req
	call.Ret = ret
	var err error
	if server == "" {
		err = fmt.Errorf("server is emtpty")
		call.Service = server
		call.Error = err
		call.done()
		return call
	}
	call.Module, call.Method, call.Error = this.parseMoudleFunc(moduleFunc)
	if err != nil {
		call.done()
		return call
	}
	this.sendCall(call)
	return call
}

// send msg 就是类似于MQ
func (this *Client) Send(h *header.Header, v any) error {
	return this.send(h, v)
}

func (this *Client) send(h *header.Header, v any) (err error) {
	this.l.Lock()
	defer this.l.Unlock()
	if !this.connecting {
		err = fmt.Errorf("%w ,client is connecting:%v", WriteError, this.connecting)
		return
	}
	if this.codec == nil {
		err = fmt.Errorf("%w,codec is nil", WriteError)
		return
	}
	err = this.codec.Write(h, v)
	return
}

func (this *Client) Marshal(v any) ([]byte, error) {
	this.l.Lock()
	defer this.l.Unlock()
	return this.codec.Marshal(v)
}

func (this *Client) Unmarshal(data *[]byte, v any) error {
	this.l.Unlock()
	defer this.l.Unlock()
	return this.codec.Unmarshal(data, v)
}

func (this *Client) sendCall(call *Call) {
	if call == nil {
		return
	}
	this.l.Lock()
	atomic.AddUint64(&this.seq, 1)
	seq := this.seq
	this.pending[seq] = call
	defer this.l.Unlock()
	h := header.Get()
	defer func() {
		header.Release(h)
	}()
	h.Init(headertype.Req, this.name, call.Service, call.Module, call.Method, seq)
	if err := this.send(h, call.Req); err != nil {
		this.l.Lock()
		call = this.pending[seq]
		delete(this.pending, seq)
		this.l.Unlock()
		if call != nil {
			err = fmt.Errorf("%w,err:%v", WriteError, err)
			call.Error = err
			call.done()
		}
	}
}
