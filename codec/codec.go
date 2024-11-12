package codec

import (
	"bufio"
	"fmt"
	"hash/crc32"
	"io"

	"github.com/ndsky1003/crpc/v2/coder"
	"github.com/ndsky1003/crpc/v2/compressor"
	"github.com/ndsky1003/crpc/v2/header"
)

// 编解码器
type Codec interface {
	WriteFrame(*header.Header, any, any) error              //coder compress写任意解码器支持的对象
	WriteFrameRawData(*header.Header, []byte, []byte) error //服务器透传
	Write([]byte) error
	Flush() error

	ReadHeader() (*header.Header, error)
	ReadMeta(*header.Header, any) error
	ReadBody(*header.Header, any) error
	Read([]byte) error
	Drop(int) error

	Close() error
}

type codec struct {
	r    io.Reader
	w    io.Writer
	conn io.ReadWriteCloser
}

func NewCodec(conn io.ReadWriteCloser) Codec {
	if conn == nil {
		panic("conn is nil")
	}
	c := &codec{
		conn: conn,
		r:    bufio.NewReader(conn),
		w:    bufio.NewWriter(conn),
	}
	return c
}

func (this *codec) WriteFrame(h *header.Header, meta, body any) (err error) {
	var metaData, bodyData, headerData []byte
	if metaData, err = coder.Marshal(h.MetaCoderT, meta); err != nil {
		err = fmt.Errorf("%w,%v", WriteError, err)
		return
	}

	if bodyData, err = coder.Marshal(h.ReqCoderT, body); err != nil {
		err = fmt.Errorf("%w,%v", WriteError, err)
		return
	}

	if bodyData, err = compressor.Zip(h.CompressT, bodyData); err != nil {
		err = fmt.Errorf("%w,%v", WriteError, err)
		return
	}

	h.MetaLen = uint32(len(metaData))
	h.Checksum = crc32.ChecksumIEEE(bodyData)
	h.BodyLen = uint64(len(bodyData))

	headerData = h.Marshal()
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

func (this *codec) ReadMeta(h *header.Header, meta any) (err error) {
	if meta == nil {
		return this.Drop(int(h.MetaLen))
	}
	data := make([]byte, h.MetaLen)
	if err = this.Read(data); err != nil {
		return
	}
	if err = coder.Unmarshal(h.MetaCoderT, data, meta); err != nil {
		return fmt.Errorf("%w,err:%v", ReadError, err)
	}
	return
}

func (this *codec) ReadBody(h *header.Header, body any) (err error) {
	length := h.BodyLen
	if body == nil {
		return this.Drop(int(length))
	}
	bodyData := make([]byte, length)
	if err = this.Read(bodyData); err != nil {
		return
	}

	if h.Checksum != 0 {
		if crc32.ChecksumIEEE(bodyData) != h.Checksum {
			err = fmt.Errorf("%w,err:%v", ReadError, UnexpectedChecksumError)
			return
		}
	}

	if bodyData, err = compressor.Unzip(h.CompressT, bodyData); err != nil {
		return fmt.Errorf("%w,err:%v", ReadError, err)
	}

	if err = coder.Unmarshal(h.MetaCoderT, bodyData, body); err != nil {
		return fmt.Errorf("%w,err:%v", ReadError, err)
	}
	return
}

func (this *codec) Read(data []byte) (err error) {
	if len(data) == 0 {
		return
	}
	if err = read(this.r, data); err != nil {
		err = fmt.Errorf("%w,err:%v", ReadError, err)
		return
	}
	return
}

func (this *codec) Drop(length int) (err error) {
	if length == 0 {
		return
	}
	data := make([]byte, length)
	return this.Read(data)
}

func (this *codec) Close() error {
	return this.conn.Close()
}
