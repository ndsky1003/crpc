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

type codecFunc func(conn io.ReadWriteCloser) (codec.Codec, error)

const defaultChunksSize = 1 * 1024 * 1024 //1M 不涉及上传文件，大多都是图片，所以限制1M合理，具体项目自定义

// service - module -> func
type Client struct {
	version       uint32 //问题自身产生的caller，被别的版本caller消费
	name          string
	url           string
	secret        string
	moduleMap     sync.Map // map[string]*module
	coderType     coder.CoderType
	compressType  compressor.CompressType
	chunksSize    int
	l             sync.Mutex
	codecGenFunc  codecFunc
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
		version:       uint32(time.Now().Unix()),
		name:          name,
		url:           url,
		chunksSize:    defaultChunksSize,
		pending:       make(map[uint64]*Call),
		coderType:     coder.JSON,
		compressType:  compressor.Raw,
		checkInterval: 1,
		heartInterval: 5,
		timeout:       -1,
	}
	if name == "" {
		panic("name is empty")
	}
	if url == "" {
		panic("url is empty")
	}

	//合并属性
	opt := options.Client().Merge(opts...)
	//属性设置开始
	if opt.Secret != nil {
		c.secret = *opt.Secret
	}
	if opt.CoderType != nil {
		c.coderType = *opt.CoderType
	}
	if opt.CompressType != nil {
		c.compressType = *opt.CompressType
	}
	c.codecGenFunc = func(conn io.ReadWriteCloser) (codec.Codec, error) {
		return codec.NewCodec(conn), nil
	}
	if opt.Timeout != nil {
		c.timeout = *opt.Timeout
	}
	if opt.CheckInterval != nil {
		c.checkInterval = *opt.CheckInterval
	}

	if opt.ChunksSize != nil {
		c.chunksSize = *opt.ChunksSize
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
				logrus.Errorf("dail err:%v\n", err)
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
				time.Sleep(this.checkInterval * time.Second) //防止连上就断开，再继续连接
				continue
			}
		} else { //heart
			if !this.isStopHeart {
				h := header.Get()
				h.InitVersionType(this.version, headertype.Ping)
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
	h.InitVersionType(this.version, headertype.Verify)
	if err = codec.Write(h, verify_req{Name: this.name, Secret: this.secret}); err != nil {
		logrus.Error(err)
		return
	}
	header.Release(h)
	h, err = codec.ReadHeader()
	if err != nil {
		logrus.Error(err)
		return err
	}
	if h.Type != headertype.Verify {
		err = fmt.Errorf("%w,headertype:%d is invalid", VerifyError, h.Type)
		logrus.Error(err)
		return
	}
	var res verify_res
	if err = codec.ReadBody(&res); err != nil {
		logrus.Error(err)
		return
	}
	header.Release(h)
	if !res.Success {
		err = fmt.Errorf("%w,verify failed", VerifyError)
		logrus.Error(err)
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

func (this *Client) func_call(coderT coder.CoderType, moduleStr, method string, reqData []byte) (ret any, err error) {
	if v, ok := this.moduleMap.Load(moduleStr); !ok {
		err = fmt.Errorf("%w,module:%s is not exist", FuncError, moduleStr)
		return
	} else {
		mod := v.(*module)
		if mtype, ok := mod.methods[method]; !ok {
			err = fmt.Errorf("%w,module:%v,method:%v is not exist", FuncError, moduleStr, method)
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
			if err = this.unmarshal(coderT, &reqData, argv.Interface()); err != nil {
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
		var h *header.Header
		h, err = this.codec.ReadHeader()
		if err != nil {
			err = fmt.Errorf("%w,%v", ReadError, err)
			break
		}
		//logrus.Infof("receiveHeader:%+v", h)
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
				if _, e := this.func_call(h.GetCoderType(), h.Module, h.Method, data); e != nil {
					logrus.Error(e)
				}
			}()
		case headertype.Req, headertype.Chunks:
			var data []byte
			if err = this.codec.ReadBodyData(&data); err != nil {
				err = fmt.Errorf("%w,%v", ReadError, err)
				break
			}
			go func() {
				defer header.Release(h)
				preHeaderType := h.Type
				var v any
				if ret, e := this.func_call(h.GetCoderType(), h.Module, h.Method, data); e != nil {
					h.Type = headertype.Reply_Error
					v = e.Error()
				} else {
					h.Type = headertype.Reply_Success
					v = ret
				}
				if preHeaderType == headertype.Chunks {
					h.CoderType = this.coderType
				}
				if e := this.send(h, v); e != nil {
					logrus.Error(e)
				}
			}()
		case headertype.Reply_Success, headertype.Reply_Error: //响应
			seq := h.Seq
			var call *Call
			if this.version == h.Version {
				this.l.Lock()
				call = this.pending[seq]
				delete(this.pending, seq)
				this.l.Unlock()
			}
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
					call.Error = fmt.Errorf("%w,header:%+v  err:%v", ServerError, h, err)
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
		default:
			err = fmt.Errorf("headerType:%v,can not handle,please call author", h.Type)
			header.Release(h)
		}
	}
	logrus.Errorf("read err:%+v\n", err)
	this.stop(err)
}

func (this *Client) parseMoudleFunc(moduleFunc string) (module, function string, err error) {
	if moduleFunc == "" {
		err = fmt.Errorf("%w,moduleFunc is empty", ModuleFuncError)
		return
	}
	modulefuncs := strings.Split(moduleFunc, ".")
	if len(modulefuncs) != 2 {
		err = ModuleFuncError
		return
	}
	module, function = modulefuncs[0], modulefuncs[1]
	return

}

// 对外的方法 sync
func (this *Client) Call(server string, moduleFunc string, req, ret any) error {
	return this._call(headertype.Req, this.coderType, this.compressType, this.timeout, server, moduleFunc, req, ret)
}

func (this *Client) _call(ht headertype.Type, coderT coder.CoderType, compressT compressor.CompressType, timeout time.Duration, server string, moduleFunc string, req, ret any) error {
	if timeout > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeout*time.Second)
		defer cancel()
		call := this._go(ht, coderT, compressT, server, moduleFunc, req, ret, make(chan *Call, 1))
		select {
		case <-ctx.Done():
			return ReqTimeOutError
		case <-call.Done:
			return call.Error
		}
	} else {
		call := <-this._go(ht, coderT, compressT, server, moduleFunc, req, ret, make(chan *Call, 1)).Done
		return call.Error
	}
}

// async
func (this *Client) Go(server string, moduleFunc string, req, ret any, done chan *Call) *Call {
	return this._go(headertype.Req, this.coderType, this.compressType, server, moduleFunc, req, ret, done)
}

func (this *Client) _go(ht headertype.Type, coderT coder.CoderType, compressT compressor.CompressType, server string, moduleFunc string, req, ret any, done chan *Call) *Call {
	call := &Call{}
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
	if server == "" {
		call.Error = fmt.Errorf("server is emtpty")
		call.done()
		return call
	}
	call.Service = server
	call.Module, call.Method, call.Error = this.parseMoudleFunc(moduleFunc)
	if call.Error != nil {
		call.done()
		return call
	}
	this.sendCall(ht, coderT, compressT, call)
	return call
}

// send msg 就是类似于MQ
func (this *Client) Send(server, moduleFunc string, v any) error {
	if server == "" {
		return errors.New("server is empty")
	}

	module, method, err := this.parseMoudleFunc(moduleFunc)
	if err != nil {
		return err
	}
	if module == "" {
		return errors.New("module is empty")
	}
	if method == "" {
		return errors.New("method is empty")
	}
	h := header.Get()
	h.InitData(this.version, headertype.Msg, this.coderType, this.compressType, this.name, server, module, method, 0)
	defer h.Release()
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

// lock prevent code = nil
func (this *Client) marshal(coderT coder.CoderType, v any) ([]byte, error) {
	this.l.Lock()
	defer this.l.Unlock()
	return this.codec.Marshal(coderT, v)
}

func (this *Client) unmarshal(coderT coder.CoderType, data *[]byte, v any) error {
	this.l.Lock()
	defer this.l.Unlock()
	return this.codec.Unmarshal(coderT, data, v)
}

func (this *Client) sendCall(ht headertype.Type, coderT coder.CoderType, compressT compressor.CompressType, call *Call) {
	if call == nil {
		return
	}
	this.l.Lock()
	atomic.AddUint64(&this.seq, 1)
	seq := this.seq
	this.pending[seq] = call
	this.l.Unlock()
	h := header.Get()
	defer func() {
		header.Release(h)
	}()
	h.InitData(this.version, ht, coderT, compressT, this.name, call.Service, call.Module, call.Method, seq)
	//logrus.Infof("header:%+v", h)
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
