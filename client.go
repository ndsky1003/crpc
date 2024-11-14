package crpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ndsky1003/crpc/v2/codec"
	"github.com/ndsky1003/crpc/v2/coder"
	"github.com/ndsky1003/crpc/v2/comm"
	"github.com/ndsky1003/crpc/v2/compressor"
	"github.com/ndsky1003/crpc/v2/header"
	"github.com/ndsky1003/crpc/v2/header/headertype"
	"github.com/sirupsen/logrus"
)

type codecFunc func(conn io.ReadWriteCloser) (codec.Codec, error)

const defaultChunksSize = 1 * 1024 * 1024 //1M 不涉及上传文件，大多都是图片，所以限制1M合理，具体项目自定义

// 1. service - func_module -> anonymity_func
// 1. service - module -> func
type Client struct {
	version      uint32 //问题自身产生的caller，被别的版本caller消费,版本1发送了一个call ,重启后版本2会消费这个call，会出现问题,因为seq是相等的
	name         string
	url          string
	moduleMap    sync.Map // map[string]*module
	l            sync.Mutex
	codecGenFunc codecFunc
	codec        codec.Codec
	seq          uint64
	pending      map[uint64]*Call
	opt          *option
	connecting   bool // client is connecting
}

func Dial(name, url string, opts ...*option) *Client {
	c := &Client{
		version: uint32(time.Now().Unix()),
		name:    name,
		url:     url,
		pending: make(map[uint64]*Call),
	}
	if name == "" {
		panic("crpc Dail name is empty")
	}
	if url == "" {
		panic("crpc Dail url is empty")
	}
	c.opt = Option().
		SetWeight(10).
		SetMetaCoderT(coder.JSON).
		SetReqCoderT(coder.JSON).
		SetResCoderT(coder.JSON).
		SetCompressT(compressor.Raw).
		SetTimeout(10).
		SetCheckInterval(1).
		SetHeartInterval(10).
		SetChunksMaxSize(defaultChunksSize).Merge(opts...)

	//合并属性
	c.codecGenFunc = func(conn io.ReadWriteCloser) (codec.Codec, error) {
		return codec.NewCodec(conn), nil
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
				time.Sleep(*this.opt.CheckInterval * time.Second)
				continue
			}
			codec, err := this.codecGenFunc(conn)
			if err != nil {
				logrus.Errorf("codec err:%v\n", err)
				time.Sleep(*this.opt.CheckInterval * time.Second)
				continue
			} else {
				if err := this.serve(codec); err != nil {
					logrus.Error("server:", err)
				}
				time.Sleep(*this.opt.CheckInterval * time.Second) //防止连上就断开，再继续连接
				continue
			}
		} else { //heart
			heat_interval := *this.opt.HeartInterval
			if heat_interval > 0 {
				func() {
					h := header.Get()
					defer h.Release()
					h.SetVersion(this.version).
						SetType(headertype.Ping).
						SetMetaCoderT(coder.JSON).
						SetReqCoderT(coder.JSON).
						SetResCoderT(coder.JSON).
						SetCompressT(compressor.Raw)
					if err := this.send(h, nil, nil); err != nil {
						logrus.Error(err)
						if errors.Is(err, io.ErrShortWrite) || errors.Is(err, WriteError) || errors.Is(err, codec.WriteError) {
							this.stop(err)
						}
					}
				}()
				time.Sleep(heat_interval * time.Second)
			} else {
				time.Sleep(*this.opt.CheckInterval * time.Second) //下次去尝试连接
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
	//verify
	h := header.Get()
	var secret string
	if v := this.opt.Secret; v != nil {
		secret = *v
	}

	h.SetVersion(this.version).SetType(headertype.Verify).
		SetMetaCoderT(coder.JSON).
		SetReqCoderT(coder.JSON).
		SetResCoderT(coder.JSON).
		SetCompressT(compressor.Raw)

	if err = codec.WriteFrame(h, nil, verify_req{Name: this.name, Weight: *this.opt.Weight, Secret: secret}); err != nil {
		logrus.Error(err)
		return
	}
	h.Release()

	if h, err = codec.ReadHeader(); err != nil {
		logrus.Error(err)
		return err
	}

	if h.Type != headertype.Res_Success {
		err = fmt.Errorf("%w,headertype:%d is invalid", VerifyError, h.Type)
		return
	}

	if _, err = codec.ReadMetaData(h); err != nil {
		err = fmt.Errorf("%w,read meta err:%w", VerifyError, err)
		return
	}

	var bodyData []byte
	if bodyData, err = codec.ReadBodyData(h); err != nil {
		err = fmt.Errorf("%w,read body err:%w", VerifyError, err)
		return
	}

	var res verify_res
	if err = coder.Unmarshal(h.ResCoderT, bodyData, &res); err != nil {
		err = fmt.Errorf("%w,verify bodyData Unmarshal err:%w", VerifyError, err)
		return
	}
	h.Release()

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
		call.Err = err
		call.done()
	}
	if this.connecting && this.codec != nil {
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
	this.opt.SetHeartInterval(-1)
}

// 内部调用
func (this *Client) func_call_local(moduleStr, method string, req any, ret any, opt *option) (err error) {
	if v, ok := this.moduleMap.Load(moduleStr); !ok {
		err = fmt.Errorf("%w,module:%s is not exist", FuncError, moduleStr)
		return
	} else {
		mod := v.(*module)
		if mtype, ok := mod.methods[method]; !ok {
			err = fmt.Errorf("%w,module:%v,method:%v is not exist", FuncError, moduleStr, method)
			return
		} else {
			in := make([]reflect.Value, 0, 3)
			if !mtype.is_func {
				in = append(in, mod.rcvr)
			}
			if (!mtype.is_func && mtype.ArgsNum == 3) || (mtype.is_func && mtype.ArgsNum == 2) {
				metav := reflect.ValueOf(opt.Meta)
				if !metav.IsValid() {
					metaIsValue := false
					if mtype.MetaType.Kind() == reflect.Pointer {
						metav = reflect.New(mtype.MetaType.Elem())
					} else {
						metav = reflect.New(mtype.MetaType)
						metaIsValue = true
					}
					if metaIsValue {
						metav = metav.Elem()
					}
				}
				in = append(in, metav)
			}
			argv := reflect.ValueOf(req)
			if !argv.IsValid() {
				argIsValue := false
				if mtype.ArgType.Kind() == reflect.Pointer {
					argv = reflect.New(mtype.ArgType.Elem())
				} else {
					argv = reflect.New(mtype.ArgType)
					argIsValue = true
				}
				if argIsValue {
					argv = argv.Elem()
				}
			}
			in = append(in, argv)
			function := mtype.method.Func

			func() { //有可能传入的参数和调用参数类型不一致
				defer func() {
					if recover_err := recover(); recover_err != nil {
						var ok1 bool
						err, ok1 = recover_err.(error)
						if !ok1 {
							err = fmt.Errorf("%v", recover_err)
						}
						err = fmt.Errorf("panic:%w ,stack:%v", err, string(debug.Stack()))
						fmt.Println(err)
					}
				}()
				returnValues := function.Call(in)
				errInter := returnValues[len(returnValues)-1].Interface()
				if errInter != nil {
					err = errInter.(error)
					return
				}
				if ret != nil && mtype.RetNum == 2 {
					if retv := reflect.ValueOf(ret); retv.IsValid() && retv.Type().Kind() == reflect.Pointer {
						retv = retv.Elem()
						if real_ret := returnValues[0]; real_ret.IsValid() {
							if real_ret.Type().Kind() == reflect.Pointer {
								real_ret = real_ret.Elem()
							}
							retv.Set(real_ret)
						}
					}
				}
			}()
			return
		}
	}
}

func (this *Client) func_call(h *header.Header, metaData, bodyData []byte) (ret any, err error) {
	module_str, method := h.Module, h.Method
	if v, ok := this.moduleMap.Load(module_str); !ok {
		err = fmt.Errorf("%w,module:%s is not exist", FuncError, module_str)
		return
	} else {
		mod := v.(*module)
		if mtype, ok := mod.methods[method]; !ok {
			err = fmt.Errorf("%w,module:%v,method:%v is not exist", FuncError, module_str, method)
			return
		} else {
			in := make([]reflect.Value, 0, 3)
			if !mtype.is_func {
				in = append(in, mod.rcvr)
			}
			if (!mtype.is_func && mtype.ArgsNum == 3) || (mtype.is_func && mtype.ArgsNum == 2) {
				var metav reflect.Value
				metaIsValue := false
				if mtype.MetaType.Kind() == reflect.Pointer {
					metav = reflect.New(mtype.MetaType.Elem())
				} else {
					metav = reflect.New(mtype.MetaType)
					metaIsValue = true
				}
				if err = coder.Unmarshal(h.MetaCoderT, metaData, metav.Interface()); err != nil {
					return
				}
				if metaIsValue {
					metav = metav.Elem()
				}
				in = append(in, metav)
			}
			var argv reflect.Value
			argIsValue := false
			if mtype.ArgType.Kind() == reflect.Pointer {
				argv = reflect.New(mtype.ArgType.Elem())
			} else {
				argv = reflect.New(mtype.ArgType)
				argIsValue = true
			}
			if err = coder.Unmarshal(h.ReqCoderT, bodyData, argv.Interface()); err != nil {
				return
			}
			if argIsValue {
				argv = argv.Elem()
			}
			in = append(in, argv)
			function := mtype.method.Func

			returnValues := function.Call(in)

			errReturn := returnValues[len(returnValues)-1]
			errInter := errReturn.Interface()
			if errInter != nil {
				err = errInter.(error)
			} else {
				if len(returnValues) == 2 {
					retValue := returnValues[0]
					ret = retValue.Interface()
				}
			}
			return
		}
	}
}

func (this *Client) input(codec codec.Codec) {
	var err error
	var h *header.Header
	for err == nil {
		if h, err = codec.ReadHeader(); err != nil {
			err = fmt.Errorf("%w,%v", ReadError, err)
			break
		}

		var metaData, bodyData []byte
		if /*h.Type&headertype.Res == 0 &&*/ h.Type.IsReq() {
			if metaData, err = codec.ReadMetaData(h); err != nil {
				err = fmt.Errorf("%w,%v", ServerError, err)
				break
			}
		}

		if bodyData, err = codec.ReadBodyData(h); err != nil {
			err = fmt.Errorf("%w,%v", ServerError, err)
			break
		}

		go func(h *header.Header, metaData, bodyData []byte) {
			defer h.Release()
			defer func() {
				if err := recover(); err != nil {
					if h.Type == headertype.Req || h.Type == headertype.Chunks {
						h.Type = headertype.Res_Err_Standard
						if e := this.send(h, nil, fmt.Errorf("%v", err)); e != nil {
							logrus.Error(e)
						}
					}
					logrus.Error(err)
				}
			}()
			if err := this.handle_msg(h, metaData, bodyData); err != nil {
				logrus.Errorf("err:%v,header:%v\n", err, h)
			}
		}(h, metaData, bodyData)
	}
	if h != nil && h.Type.IsReq() { //req
		h.Type = headertype.Res_Err_Standard
		if err := this.send(h, nil, err); err != nil {
			logrus.Error(err)
		}
	}
	time.Sleep(1e5) //上一个消息尚未处理完,下一个消息就报错退出了,call有概率会拿到第二个的错误消息,应为第二个处理的更快,这里加一个临界值
	logrus.Errorf("%v read err:%+v\n", this.name, err)
	this.stop(err)
}

func (this *Client) handle_msg(h *header.Header, metaData, bodyData []byte) (err error) {
	switch h.Type {
	case headertype.Msg:
		if _, e := this.func_call(h, metaData, bodyData); e != nil {
			logrus.Error(e)
		}
	case headertype.Req, headertype.Chunks:
		var v any
		if ret, e := this.func_call(h, metaData, bodyData); e != nil {
			if comm.IsStandardErr(e) {
				h.Type = headertype.Res_Err_Standard
				v = e
			} else {
				h.Type = headertype.Res_Err_Custom
				v = e
			}
		} else {
			h.Type = headertype.Res_Success
			v = ret
		}
		if e := this.send(h, nil, v); e != nil {
			logrus.Error(e)
		}
	case headertype.Res_Success, headertype.Res_Err_Standard, headertype.Res_Err_Custom: //响应
		seq := h.Seq
		var call *Call
		if this.version == h.Version {
			this.l.Lock()
			call = this.pending[seq]
			delete(this.pending, seq)
			this.l.Unlock()
		}
		if call != nil {
			switch {
			case h.Type == headertype.Res_Err_Standard:
				call.Err = errors.New(string(bodyData))
				call.done()
			case h.Type == headertype.Res_Err_Custom:
				if call.opt.RetErr != nil {
					err = coder.Unmarshal(h.GetMarshalType(), bodyData, call.opt.RetErr)
					if err != nil {
						call.Err = fmt.Errorf("unmarshal body error,%w", err)
					} else {
						call.Err = call.opt.RetErr
					}
				} else {
					call.Err = ErrCusstomNoReceiveType
				}
				call.done()
			default:
				err = coder.Unmarshal(h.GetMarshalType(), bodyData, call.Ret)
				if err != nil {
					call.Err = fmt.Errorf("%v unmarshal body error,%w", this.name, err)
				}
				call.done()
			}
		}
	case headertype.Pong:
	default:
		err = fmt.Errorf("headerType:%v,can not handle,please call author", h.Type)
	}
	return
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
func (this *Client) Call(server string, moduleFunc string, req, ret any, opts ...*option) error {
	opt := Option().Merge(this.opt).Merge(opts...)
	if server == this.name {
		module, method, err := this.parseMoudleFunc(moduleFunc)
		if err != nil {
			return err
		}
		return this.func_call_local(module, method, req, ret, opt)
	}
	return this._call(headertype.Req, server, moduleFunc, req, ret, opt)
}

func (this *Client) _call(ht headertype.T, server string, moduleFunc string, req, ret any, opt *option) error {
	timeout := *opt.Timeout
	if timeout > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeout*time.Second)
		defer cancel()
		call := this._go(ht, server, moduleFunc, req, ret, make(chan *Call, 1), opt)
		select {
		case <-ctx.Done():
			return ReqTimeOutError
		case <-call.Done:
			return call.Err
		}
	} else {
		call := <-this._go(ht, server, moduleFunc, req, ret, make(chan *Call, 1), opt).Done
		return call.Err
	}
}

// async
func (this *Client) Go(server string, moduleFunc string, req, ret any, opts ...*option) *Call {
	opt := Option().Merge(this.opt).Merge(opts...)
	return this._go(headertype.Req, server, moduleFunc, req, ret, make(chan *Call, 1), opt)
}

func (this *Client) _go(ht headertype.T, server string, moduleFunc string, req, ret any, done chan *Call, opt *option) *Call {
	call := &Call{}
	if done == nil {
		done = make(chan *Call, 10) // buffered.
	} else {
		if cap(done) == 0 {
			panic("crpc: done channel is unbuffered")
		}
	}
	call.Done = done
	call.Req = req
	call.Ret = ret
	if server == "" {
		call.Err = fmt.Errorf("server is emtpty")
		call.done()
		return call
	}
	call.Service = server
	call.Module, call.Method, call.Err = this.parseMoudleFunc(moduleFunc)
	if call.Err != nil {
		call.done()
		return call
	}
	this.sendCall(ht, call, opt)
	return call
}

// send msg 就是类似于MQ
func (this *Client) Send(server, moduleFunc string, v any, opts ...*option) error {
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
	opt := Option().Merge(this.opt).Merge(opts...)
	h := header.Get()
	h.SetVersion(this.version).
		SetType(headertype.Msg).
		SetMetaCoderT(*opt.MetaCoderT).
		SetReqCoderT(*opt.ReqCoderT).
		SetResCoderT(*opt.ResCoderT).
		SetCompressT(*opt.CompressT).
		SetFromService(this.name).
		SetToService(server).
		SetModule(module).
		SetMethod(method)
	defer h.Release()
	return this.send(h, opt.Meta, v)
}

func (this *Client) send(h *header.Header, meta, body any) (err error) {
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
	return this.codec.WriteFrame(h, meta, body)
}

func (this *Client) sendCall(ht headertype.T, call *Call, opt *option) {
	if call == nil {
		return
	}
	seq := atomic.AddUint64(&this.seq, 1) // this.seq
	this.l.Lock()
	this.pending[seq] = call
	this.l.Unlock()
	h := header.Get()
	defer h.Release()
	// h.InitData(this.version, ht, *opt.CoderType, *opt.CompressType, this.name, call.Service, call.Module, call.Method, seq)
	h.SetVersion(this.version).
		SetType(ht).
		SetMetaCoderT(*opt.MetaCoderT).
		SetReqCoderT(*opt.ReqCoderT).
		SetResCoderT(*opt.ResCoderT).
		SetCompressT(*opt.CompressT).
		SetFromService(this.name).
		SetToService(call.Service).
		SetModule(call.Module).
		SetMethod(call.Method).
		SetSeq(seq)
	if err := this.send(h, opt.Meta, call.Req); err != nil {
		this.l.Lock()
		call = this.pending[seq]
		delete(this.pending, seq)
		this.l.Unlock()
		if call != nil {
			err = fmt.Errorf("%w,err:%v", WriteError, err)
			call.Err = err
			call.done()
		}
	}
}
