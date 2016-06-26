package gsync

import (
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"path"
	"strings"
)

func normpath(p string) string {
	return strings.Replace(p, "\\", "/", -1)
}

type Request struct {
	Hashes map[string]string
	Ignore []string
}

func makeRequest(stripdir string, curdir string, req *Request, recursive bool) error {
	fis, err := ioutil.ReadDir(curdir)
	if err != nil {
		return err
	}
	if req.Hashes == nil {
		req.Hashes = make(map[string]string)
	}
	for _, fi := range fis {
		fiPath := normpath(path.Join(curdir, fi.Name()))
		if fi.IsDir() {
			if recursive {
				if err = makeRequest(stripdir, fiPath, req, recursive); err != nil {
					return err
				}
			}
			continue
		}

		contents, err := ioutil.ReadFile(fiPath)
		if err != nil {
			return fmt.Errorf("read file %s err:%v", fiPath, err)
		}
		fiPath = strings.Replace(fiPath, stripdir, "", -1)
		req.Hashes[fiPath] = fmt.Sprintf("%x", md5.Sum(contents))
	}

	return nil

}

func MakeRequest(dir string, recursive bool) (*Request, error) {
	req := &Request{
		Hashes: make(map[string]string),
	}
	dir = normpath(path.Clean(dir))
	if !strings.HasSuffix(dir, "/") {
		dir = dir + "/"
	}
	err := makeRequest(dir, dir, req, true)
	return req, err
}
