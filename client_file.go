package crpc

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ndsky1003/crpc/v2/coder"
	"github.com/ndsky1003/crpc/v2/compressor"
	"github.com/ndsky1003/crpc/v2/dto"
	"github.com/ndsky1003/crpc/v2/header/headertype"
)

func (this *Client) SendFile(server string, moduleFunc string, save_path string, reader io.Reader, opts ...*Option) (err error) {
	if save_path == "" {
		return errors.New("filename not empty")
	}

	if filepath.IsAbs(save_path) {
		return errors.New("filename must relative path")
	}

	opt := Options().Merge(this.opt).Merge(opts...).
		SetReqCoderT(coder.FilePack).
		SetCompressT(compressor.Raw).
		SetTimeout(60 * 60 * 2)
	chunks_size := *opt.ChunksSize

	data := make([]byte, chunks_size)
	var chunkIndex uint16 = 0
	filebody := &dto.FileBody{
		Filename: save_path,
	}

	for {
		var n int
		n, err = reader.Read(data)
		if err != nil {
			break
		}
		filebody.ChunksIndex = chunkIndex
		filebody.Data = data[:n]
		if err := this._call(headertype.Chunks, server, moduleFunc, filebody, nil, opt); err != nil {
			return err
		}
		if n < chunks_size {
			break
		}
		filebody.Offset += uint64(n)
		chunkIndex++
	}
	if err == nil {
		filebody.IsFinish = 1
		filebody.Offset = 0
		clear(filebody.Data)
		if err = this._call(headertype.Req, server, moduleFunc, filebody, nil, opt); err != nil {
			return
		}
	}
	return
}

var exe = filepath.Base(os.Args[0])
var tmp_dir = ".tmp"

func init() {
	os.Mkdir(".tmp", 0700)
}

func WriteFile(req *dto.FileBody) (err error) {
	flag := os.O_CREATE | os.O_APPEND | os.O_WRONLY
	dir, file := filepath.Split(req.Filename)
	tmp_file := fmt.Sprintf("%s/%v/%v.tmp", tmp_dir, exe, file)
	if req.IsFinish == 1 {
		err = os.Rename(tmp_file, req.Filename)
		return
	}
	if req.ChunksIndex == 0 {
		if dir != "" {
			if err = os.MkdirAll(dir, 0700); err != nil {
				return
			}
		}
		flag |= os.O_TRUNC
		tmp_dir := fmt.Sprintf("%s/%v", tmp_dir, exe)
		if err = os.MkdirAll(tmp_dir, 0700); err != nil {
			return
		}
	}
	f, err := os.OpenFile(tmp_file, flag, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	_, err = f.Write(req.Data)
	return
}
