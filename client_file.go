package crpc

import (
	"errors"
	"io"
	"path/filepath"

	"github.com/ndsky1003/crpc/coder"
	"github.com/ndsky1003/crpc/compressor"
	"github.com/ndsky1003/crpc/dto"
	"github.com/ndsky1003/crpc/header/headertype"
)

func (this *Client) SendFile(server string, moduleFunc string, filename string, reader io.Reader) error {
	if filename == "" {
		return errors.New("filename not empty")
	}

	if filepath.IsAbs(filename) {
		return errors.New("filename must relative path")
	}

	data := make([]byte, this.chunksSize)
	var chunkIndex uint16 = 0
	filebody := &dto.FileBody{
		Filename: filename,
	}
	for {
		n, err := reader.Read(data)
		if err != nil {
			return err
		}
		filebody.ChunksIndex = chunkIndex
		filebody.Data = data[:n]
		if err := this._call(headertype.Chunks, coder.FilePack, compressor.Raw, 60*60*2, server, moduleFunc, filebody, nil); err != nil {
			return err
		}
		if n < this.chunksSize {
			return nil
		}
		filebody.Offset += uint64(n)
		chunkIndex++
	}
}
