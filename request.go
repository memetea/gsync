package gsync

import (
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/djherbis/times"
)

func normpath(p string) string {
	return strings.Replace(p, "\\", "/", -1)
}

func fexists(p string) bool {
	_, err := os.Stat(p)
	if err == nil || !os.IsNotExist(err) {
		return true
	}
	return false
}

func copyFile(from string, to string, overwrite bool) error {
	if from == to {
		return nil
	}
	toexists := fexists(to)
	if !overwrite && toexists {
		return fmt.Errorf("file exists:%s", to)
	}
	frsrc, err := os.Open(from)
	if err != nil {
		return err
	}
	defer frsrc.Close()
	err = os.MkdirAll(filepath.Dir(to), 0777)
	if err != nil {
		return err
	}
	var frdst *os.File
	if !toexists {
		frdst, err = os.Create(to)
		if err != nil {
			return err
		}
	} else {
		frdst, err = os.Open(to)
		if err != nil {
			return err
		}
		frdst.Truncate(0)
	}
	_, err = io.Copy(frdst, frsrc)
	if err != nil {
		return err
	}
	frdst.Close()
	fi, err := os.Stat(from)
	if err != nil {
		return err
	}
	timeSpec := times.Get(fi)
	if err = os.Chmod(to, fi.Mode()); err != nil {
		return err
	}
	return os.Chtimes(to, timeSpec.AccessTime(), timeSpec.ModTime())
}

func copyDir(fromdir string, todir string, overwrite bool) error {
	fromdir = filepath.Clean(fromdir)
	todir = filepath.Clean(todir)
	if !overwrite {
		err := filepath.Walk(fromdir, func(path string, info os.FileInfo, err error) error {
			fpath := filepath.Join(todir, path)
			if !overwrite && fexists(fpath) {
				return fmt.Errorf("file exists:%s", fpath)
			}
			return nil
		})
		return err
	}

	err := filepath.Walk(fromdir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			if strings.HasSuffix(fromdir, info.Name()) {
				return nil
			}
			os.MkdirAll(path, 0777)
			return nil
		}
		fto := strings.Replace(filepath.Dir(path), fromdir, todir, 1)
		fto = filepath.Join(fto, info.Name())
		return copyFile(path, fto, overwrite)
	})
	return err
}

type Request struct {
	Hashes map[string]string
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

//FilterIgnore filter out files match the ignore patterns
func FilterIgnore(req *Request, patterns []string) {
	for k := range req.Hashes {
		for _, pattern := range patterns {
			if ok, err := filepath.Match(pattern, k); ok && err == nil {
				delete(req.Hashes, k)
			}
		}
	}
}
