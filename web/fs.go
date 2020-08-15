package web

import (
	"io"
	"net/http"
	"os"
)

// Credit: https://stackoverflow.com/questions/49589685/good-way-to-disable-directory-listing-with-http-fileserver-in-go
type justFilesFilesystem struct {
	fs http.FileSystem
	// readDirBatchSize - configuration parameter for `Readdir` func
	readDirBatchSize int
}

func (fs justFilesFilesystem) Open(name string) (http.File, error) {
	f, err := fs.fs.Open(name)
	if err != nil {
		return nil, err
	}
	return neuteredStatFile{File: f, readDirBatchSize: fs.readDirBatchSize}, nil
}

type neuteredStatFile struct {
	http.File
	readDirBatchSize int
}

func (e neuteredStatFile) Stat() (os.FileInfo, error) {
	s, err := e.File.Stat()
	if err != nil {
		return nil, err
	}
	if s.IsDir() {
	LOOP:
		for {
			fl, err := e.File.Readdir(e.readDirBatchSize)
			switch err {
			case io.EOF:
				break LOOP
			case nil:
				for _, f := range fl {
					if f.Name() == "index.html" {
						return s, err
					}
				}
			default:
				return nil, err
			}
		}
		return nil, os.ErrNotExist
	}
	return s, err
}
