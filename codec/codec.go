package codec

import (
	"bufio"
	"fmt"
	"hash/crc32"
	"io"

	"github.com/ndsky1003/crpc/coder"
	"github.com/ndsky1003/crpc/compressor"
	"github.com/ndsky1003/crpc/header"
	"github.com/ndsky1003/crpc/options"
	"github.com/ndsky1003/crpc/serializer"
)

// 编解码器
type Codec interface {
	Write(*header.Header, any) error //写任意解码器支持的对象
	WriteData(*header.Header, []byte) error
	WriteRawData(*header.Header, []byte) error //服务器转发或者，发送文件 不需要数据处理

	ReadHeader() (*header.Header, error)
	ReadBody(any) error
	ReadBodyData(*[]byte) error
	ReadBodyRawData(*[]byte) error //服务器转发或者，发送文件 不需要数据处理

	Close() error

	Marshal(any) ([]byte, error)
	Unmarshal(*[]byte, any) error
}

type codec struct {
	r    io.Reader
	w    io.Writer
	c    io.Closer
	conn io.ReadWriteCloser
	h    *header.Header
	//options following
	coderType             coder.CoderType
	compressType          compressor.CompressType
	serializer.Serializer //持久化工具
}

func NewCodec(conn io.ReadWriteCloser, opts ...*options.CodecOptions) Codec {
	if conn == nil {
		panic("conn is nil")
	}
	var c = &codec{
		conn:         conn,
		r:            bufio.NewReader(conn),
		w:            bufio.NewWriter(conn),
		c:            conn,
		coderType:    coder.JSON,
		compressType: compressor.Raw,
	}
	opt := options.CodecOptions{}
	opt.Merge(opts...)

	if opt.CoderType != nil {
		c.coderType = *opt.CoderType
	}
	if opt.CoderType != nil {
		c.compressType = *opt.CompressType
	}
	if opt.Serializer != nil {
		c.Serializer = opt.Serializer
	}
	return c
}

func (this *codec) WriteData(h *header.Header, data []byte) (err error) {
	h.CoderType = this.coderType
	h.CompressType = this.compressType
	var headerData, bodyData []byte
	defer func() {
		if err != nil && this.Serializer != nil {
			if e := this.Serialize(h, bodyData); e != nil {
				fmt.Println(e, headerData, bodyData)
			}
		}
	}()
	zip, ok := compressor.Compressors[h.CompressType]
	if !ok {
		return fmt.Errorf("compressor:%d is not exist", h.CoderType)
	}
	bodyData, err = zip.Zip(data)
	if err != nil {
		return err
	}
	h.Checksum = crc32.ChecksumIEEE(bodyData)
	h.BodyLen = uint64(len(bodyData))
	return this.WriteRawData(h, bodyData)
}

// MARK 服务器转发使用
func (this *codec) WriteRawData(h *header.Header, bodyData []byte) (err error) {
	if err = sendFrame(this.w, h.Marshal()); err != nil {
		err = fmt.Errorf("%w,%v", WriteError, err)
		return
	}
	if err = write(this.w, bodyData); err != nil {
		err = fmt.Errorf("%w,%v", WriteError, err)
		return
	}
	if err = this.w.(*bufio.Writer).Flush(); err != nil {
		err = fmt.Errorf("%w,%v", WriteError, err)
		return
	}
	return
}

func (this *codec) Write(h *header.Header, body any) (err error) {
	h.CoderType = this.coderType
	h.CompressType = this.compressType
	coder, ok := coder.Coders[h.CoderType]
	if !ok {
		return fmt.Errorf("coder:%d is not exist", h.CoderType)
	}
	reqBody, err := coder.Marshal(body)
	if err != nil {
		return err
	}
	return this.WriteData(h, reqBody)
}

func (this *codec) ReadHeader() (*header.Header, error) {
	data, err := recvFrame(this.r)
	if err != nil {
		return nil, fmt.Errorf("%w,err:%v", ReadError, err)
	}
	h := header.Get()
	if err = h.Unmarshal(data); err != nil {
		return nil, fmt.Errorf("%w,err:%v", ReadError, err)
	}
	this.h = h
	return this.h, nil
}

func (this *codec) ReadBodyRawData(data *[]byte) (err error) {
	bodyLen := this.h.BodyLen
	body := make([]byte, bodyLen)
	if err = read(this.r, body); err != nil {
		err = fmt.Errorf("%w,err:%v", ReadError, err)
		return
	}
	if data != nil {
		*data = body
	}
	return
}

func (this *codec) ReadBodyData(data *[]byte) (err error) {
	var body []byte
	if err = this.ReadBodyRawData(&body); err != nil {
		return
	}
	if data == nil {
		return
	}
	if this.h.Checksum != 0 {
		if crc32.ChecksumIEEE(body) != this.h.Checksum {
			err = fmt.Errorf("%w,err:%v", ReadError, UnexpectedChecksumError)
			return
		}
	}
	unzip, ok := compressor.Compressors[this.h.CompressType]
	if !ok {
		return fmt.Errorf("%w,compressor:%d is not exist", ReadError, this.h.CompressType)
	}
	*data, err = unzip.Unzip(body)
	if err != nil {
		err = fmt.Errorf("%w,err:%v", ReadError, err)
		return
	}
	return
}

func (this *codec) ReadBody(v any) (err error) {
	if v == nil {
		if err = this.ReadBodyData(nil); err != nil {
			return
		}
	} else {
		var data []byte
		if err = this.ReadBodyData(&data); err != nil {
			return
		}
		err = this.Unmarshal(&data, v)
	}
	return
}

func (this *codec) Marshal(v any) (data []byte, err error) {
	coder, ok := coder.Coders[this.h.CoderType]
	if !ok {
		err = fmt.Errorf("%w,coder:%d is not exist", WriteError, this.h.CoderType)
		return
	}
	data, err = coder.Marshal(v)
	if err != nil {
		err = fmt.Errorf("%w,coder:%d marshal err:%v", WriteError, this.h.CoderType, err)
	}
	return
}

func (this *codec) Unmarshal(data *[]byte, v any) error {
	coder, ok := coder.Coders[this.h.CoderType]
	if !ok {
		return fmt.Errorf("%w,coder:%d is not exist", ReadError, this.h.CoderType)
	}
	if data == nil {
		return fmt.Errorf("%w,data is nil", ReadError)
	}
	if err := coder.Unmarshal(*data, v); err != nil {
		return fmt.Errorf("%w,coder unmarshal err:%v", ReadError, err)
	}
	return nil
}

func (this *codec) Close() error {
	return this.c.Close()
}
