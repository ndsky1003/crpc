package crpc

import (
	"errors"
	"io"
	"path/filepath"

	"github.com/ndsky1003/crpc/v2/coder"
	"github.com/ndsky1003/crpc/v2/compressor"
	"github.com/ndsky1003/crpc/v2/dto"
	"github.com/ndsky1003/crpc/v2/header/headertype"
)

func (this *Client) SendFile(server string, moduleFunc string, save_path string, reader io.Reader, opts ...*option) error {
	if save_path == "" {
		return errors.New("filename not empty")
	}

	if filepath.IsAbs(save_path) {
		return errors.New("filename must relative path")
	}

	opt := Option().Merge(this.opt).Merge(opts...).
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
		n, err := reader.Read(data)
		if err != nil {
			return err
		}
		filebody.ChunksIndex = chunkIndex
		filebody.Data = data[:n]
		if err := this._call(headertype.Chunks, server, moduleFunc, filebody, nil, opt); err != nil {
			return err
		}
		if n < chunks_size {
			return nil
		}
		filebody.Offset += uint64(n)
		chunkIndex++
	}
}
