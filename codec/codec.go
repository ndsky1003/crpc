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
	Write(*header.Header, any) error           //coder compress写任意解码器支持的对象
	WriteData(*header.Header, []byte) error    //compress
	WriteRawData(*header.Header, []byte) error //none服务器转发或者，发送文件 不需要数据处理

	ReadHeader() (*header.Header, error)
	ReadBody(*header.Header, any) error            //coder compress
	ReadBodyData(*header.Header, *[]byte) error    //compress
	ReadBodyRawData(*header.Header, *[]byte) error //none服务器转发或者，发送文件 不需要数据处理

	Close() error

	Marshal(coder.T, any) ([]byte, error)
	Unmarshal(coder.T, *[]byte, any) error
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

func (this *codec) Write(h *header.Header, body any) error {
	if body == nil {
		return this.WriteData(h, nil)
	}

	coder, ok := coder.Coders[h.ReqCoderT]
	if !ok {
		return fmt.Errorf("coder:%d is not exist", h.ReqCoderT)
	}

	if data, err := coder.Marshal(body); err != nil {
		return err
	} else {
		return this.WriteData(h, data)
	}
}

func (this *codec) WriteData(h *header.Header, data []byte) (err error) {
	zip, ok := compressor.Compressors[h.CompressT]
	if !ok {
		return fmt.Errorf("compressor:%d is not exist", h.ReqCoderT)
	}
	bodyData, err := zip.Zip(data)
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

func (this *codec) ReadBodyRawData(h *header.Header, data *[]byte) (err error) {
	bodyLen := h.BodyLen
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

func (this *codec) ReadBodyData(h *header.Header, data *[]byte) (err error) {
	if data == nil {
		err = this.ReadBodyRawData(h, nil)
		return
	}
	var body []byte
	if err = this.ReadBodyRawData(h, &body); err != nil {
		return
	}
	if h.Checksum != 0 {
		if crc32.ChecksumIEEE(body) != h.Checksum {
			err = fmt.Errorf("%w,err:%v", ReadError, UnexpectedChecksumError)
			return
		}
	}
	unzip, ok := compressor.Compressors[h.CompressT]
	if !ok {
		return fmt.Errorf("%w,compressor:%d is not exist", ReadError, h.CompressT)
	}
	*data, err = unzip.Unzip(body)
	if err != nil {
		err = fmt.Errorf("%w,err:%v", ReadError, err)
		return
	}
	return
}

func (this *codec) ReadBody(h *header.Header, v any) (err error) {
	if v == nil {
		if err = this.ReadBodyData(h, nil); err != nil {
			return
		}
	} else {
		var data []byte
		if err = this.ReadBodyData(h, &data); err != nil {
			return
		}
		err = this.Unmarshal(h.GetCoderType(), &data, v)
	}
	return
}

func (this *codec) Marshal(coderT coder.T, v any) (data []byte, err error) {
	coder, ok := coder.Coders[coderT]
	if !ok {
		err = fmt.Errorf("%w,coder:%d is not exist", WriteError, coderT)
		return
	}
	data, err = coder.Marshal(v)
	if err != nil {
		err = fmt.Errorf("%w,coder:%d marshal err:%v", WriteError, coderT, err)
	}
	return
}

func (this *codec) Unmarshal(coderT coder.T, data *[]byte, v any) error {
	coder, ok := coder.Coders[coderT]
	if !ok {
		return fmt.Errorf("%w,coder:%d is not exist", ReadError, coderT)
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
	return this.conn.Close()
}
