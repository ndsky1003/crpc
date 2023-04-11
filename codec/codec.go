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
	Write(*header.Header, any) error
	ReadHeader() (*header.Header, error)
	ReadBody(any) error
	Close() error
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
		conn: conn,
		r:    bufio.NewReader(conn),
		w:    bufio.NewWriter(conn),
		c:    conn,
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

func (this *codec) Write(h *header.Header, body any) (err error) {
	h.CoderType = this.coderType
	h.CompressType = this.compressType
	var headerData, bodyData []byte
	defer func() {
		if err != nil && this.Serializer != nil {
			if e := this.Serialize(headerData, bodyData); e != nil {
				fmt.Println(e, headerData, bodyData)
			}
		}
	}()
	coder, ok := coder.Coders[h.CoderType]
	if !ok {
		return fmt.Errorf("coder:%d is not exist", h.CoderType)
	}
	reqBody, err := coder.Marshal(body)
	if err != nil {
		return err
	}
	zip, ok := compressor.Compressors[h.CompressType]
	if !ok {
		return fmt.Errorf("compressor:%d is not exist", h.CoderType)
	}
	bodyData, err = zip.Zip(reqBody)
	if err != nil {
		return err
	}
	h.Checksum = crc32.ChecksumIEEE(bodyData)
	h.BodyLen = uint32(len(bodyData))
	headerData = h.Marshal()
	if err = sendFrame(this.w, headerData); err != nil {
		return
	}
	if err = write(this.w, bodyData); err != nil {
		return
	}
	if err = this.w.(*bufio.Writer).Flush(); err != nil {
		return
	}
	return

}

func (this *codec) ReadHeader() (*header.Header, error) {
	data, err := recvFrame(this.r)
	if err != nil {
		return nil, err
	}
	h := header.Get()
	if err = h.Unmarshal(data); err != nil {
		return nil, err
	}
	this.h = h
	return this.h, nil
}

func (this *codec) ReadBody(v any) (err error) {
	bodyLen := this.h.BodyLen
	if v == nil {
		if bodyLen != 0 {
			if err := read(this.r, make([]byte, bodyLen)); err != nil {
				return err
			}
		}
		return nil
	}

	body := make([]byte, bodyLen)
	if err = read(this.r, body); err != nil {
		return err
	}

	if this.h.Checksum != 0 {
		if crc32.ChecksumIEEE(body) != this.h.Checksum {
			return UnexpectedChecksumError
		}
	}
	unzip, ok := compressor.Compressors[this.h.CompressType]
	if !ok {
		return fmt.Errorf("compressor:%d is not exist", this.h.CompressType)
	}
	coder, ok := coder.Coders[this.h.CoderType]
	if !ok {
		return fmt.Errorf("coder:%d is not exist", this.h.CoderType)
	}
	resp, err := unzip.Unzip(body)
	if err != nil {
		return err
	}
	return coder.Unmarshal(resp, v)
}

func (this *codec) Close() error {
	return this.c.Close()
}
