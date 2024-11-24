package codec

import (
	"bufio"
	"fmt"
	"hash/crc32"
	"io"
	"net"
	"time"

	"github.com/ndsky1003/crpc/v2/coder"
	"github.com/ndsky1003/crpc/v2/compressor"
	"github.com/ndsky1003/crpc/v2/header"
	"github.com/ndsky1003/crpc/v2/header/headertype"
)

// 编解码器
type Codec interface {
	WriteFrame(*header.Header, any, any) error              //coder compress写任意解码器支持的对象
	WriteFrameRawData(*header.Header, []byte, []byte) error //服务器透传
	Write([]byte) error
	Flush() error

	ReadHeader() (*header.Header, error)
	ReadMetaData(*header.Header) ([]byte, error)
	ReadMetaRawData(*header.Header) ([]byte, error)
	ReadBodyData(*header.Header) ([]byte, error)
	ReadBodyRawData(*header.Header) ([]byte, error)

	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	Close() error
}

type codec struct {
	r    io.Reader
	w    io.Writer
	conn net.Conn
}

func NewCodec(conn net.Conn) Codec {
	if conn == nil {
		panic("crpc conn is nil")
	}
	c := &codec{
		conn: conn,
		r:    bufio.NewReader(conn),
		w:    bufio.NewWriter(conn),
	}
	return c
}

// if meta == nil ,编码结果为空,不会进行编码,防止传递过多无用信息
// 因为json的编码nil,也会占用4个字节:null
func (this *codec) WriteFrame(h *header.Header, meta, body any) (err error) {
	var metaData, bodyData, headerData []byte
	if meta != nil {
		if metaData, err = coder.Marshal(h.MetaCoderT, meta); err != nil {
			err = fmt.Errorf("%w,%v", WriteError, err)
			return
		}
	}

	if h.Type == headertype.Res_Err_Standard {
		errstr := ""
		switch body.(type) {
		case error:
			errstr = body.(error).Error()
		case string:
			errstr = body.(string)
		default:
			errstr = fmt.Sprintf("illegal standard err: %v", body)
		}
		bodyData = []byte(errstr)
	} else {
		if bodyData, err = coder.Marshal(h.GetMarshalType(), body); err != nil {
			err = fmt.Errorf("%w,%v", WriteError, err)
			return
		}

		if bodyData, err = compressor.Zip(h.CompressT, bodyData); err != nil {
			err = fmt.Errorf("%w,%v", WriteError, err)
			return
		}
	}

	h.MetaLen = uint32(len(metaData))
	h.Checksum = crc32.ChecksumIEEE(bodyData)
	h.BodyLen = uint64(len(bodyData))

	headerData = h.Marshal()
	if err = sendFrame(this.w, headerData); err != nil {
		err = fmt.Errorf("%w,%v", WriteError, err)
		return
	}

	if meta != nil {
		if err = this.Write(metaData); err != nil {
			return
		}
	}

	if err = this.Write(bodyData); err != nil {
		return
	}

	if err = this.Flush(); err != nil {
		err = fmt.Errorf("%w,%v", WriteError, err)
		return
	}

	return
}

func (this *codec) WriteFrameRawData(h *header.Header, metaData, bodyData []byte) (err error) {

	headerData := h.Marshal()
	if err = sendFrame(this.w, headerData); err != nil {
		err = fmt.Errorf("%w,%v", WriteError, err)
		return
	}

	if err = this.Write(metaData); err != nil {
		return
	}

	if err = this.Write(bodyData); err != nil {
		return
	}

	if err = this.Flush(); err != nil {
		err = fmt.Errorf("%w,%v", WriteError, err)
		return
	}

	return
}

func (this *codec) Write(data []byte) (err error) {
	if err = write(this.w, data); err != nil {
		err = fmt.Errorf("%w,%v", WriteError, err)
		return
	}
	return
}

func (this *codec) Flush() error {
	return this.w.(*bufio.Writer).Flush()
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
	return h, nil
}

func (this *codec) ReadMetaData(h *header.Header) (data []byte, err error) {
	data = make([]byte, h.MetaLen)
	err = this.read(data)
	return
}
func (this *codec) ReadMetaRawData(h *header.Header) (data []byte, err error) {
	return this.ReadMetaData(h)
}
func (this *codec) ReadBodyRawData(h *header.Header) (data []byte, err error) {
	data = make([]byte, h.BodyLen)
	err = this.read(data)
	return
}

func (this *codec) ReadBodyData(h *header.Header) (data []byte, err error) {
	if data, err = this.ReadBodyRawData(h); err != nil {
		return
	}

	if h.Checksum != 0 {
		if crc32.ChecksumIEEE(data) != h.Checksum {
			err = fmt.Errorf("%w,err:%v", ReadError, UnexpectedChecksumError)
			return
		}
	}

	if data, err = compressor.Unzip(h.CompressT, data); err != nil {
		err = fmt.Errorf("%w,err:%v", ReadError, err)
		return
	}
	return
}

func (this *codec) read(data []byte) (err error) {
	if len(data) == 0 {
		return
	}
	if err = read(this.r, data); err != nil {
		err = fmt.Errorf("%w,err:%v", ReadError, err)
		return
	}
	return
}

func (this *codec) Close() error {
	return this.conn.Close()
}

func (this *codec) SetDeadline(t time.Time) error {
	return this.conn.SetDeadline(t)
}

func (this *codec) SetReadDeadline(t time.Time) error {
	return this.conn.SetReadDeadline(t)
}

func (this *codec) SetWriteDeadline(t time.Time) error {
	return this.conn.SetWriteDeadline(t)
}
