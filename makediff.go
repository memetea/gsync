package gsync

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

type Diff struct {
	NewHash string
	OldHash string
	NewSize int64
	Mode    os.FileMode
	ModTime time.Time
}

type DiffMap map[string]Diff

func calcDiff(stripdir, curdir string, req *Request, diffMap DiffMap) error {
	fmt.Printf("calc diff for %s\n", curdir)
	fis, err := ioutil.ReadDir(curdir)
	if err != nil {
		return err
	}
	for _, fi := range fis {
		fiPath := normpath(path.Join(curdir, fi.Name()))

		if fi.IsDir() {
			if err = calcDiff(stripdir, path.Join(curdir, fi.Name()), req, diffMap); err != nil {
				return err
			}
			continue
		}

		relPath := strings.Replace(fiPath, stripdir, "", 1)
		content, err := ioutil.ReadFile(fiPath)
		if err != nil {
			return nil
		}
		newHash := fmt.Sprintf("%x", md5.Sum(content))
		oldHash, ok := req.Hashes[relPath]
		if !ok {
			oldHash = ""
		}
		if newHash != oldHash {
			diffMap[relPath] = Diff{
				NewHash: newHash,
				OldHash: oldHash,
				NewSize: fi.Size(),
				Mode:    fi.Mode(),
				ModTime: fi.ModTime(),
			}
		}
	}
	return nil
}

func CalcDiff(cmpdir string, req *Request) (DiffMap, error) {
	diffMap := make(DiffMap)
	cmpdir = normpath(path.Clean(cmpdir))
	if !strings.HasSuffix(cmpdir, "/") {
		cmpdir = cmpdir + "/"
	}
	err := calcDiff(cmpdir, cmpdir, req, diffMap)
	return diffMap, err
}

func PrepareDiff(rootdir string, diff DiffMap) (string, error) {
	//concat hash
	var buf bytes.Buffer
	for _, v := range diff {
		buf.WriteString(v.NewHash)
	}
	fname := fmt.Sprintf("%x", md5.Sum(buf.Bytes()))
	wd, _ := os.Getwd()
	fname = path.Join(wd, "cache", fmt.Sprintf("%s.tar.gz", fname))
	_, err := os.Stat(fname)
	if err == nil || !os.IsNotExist(err) {
		//cache exists
		return fname, nil
	}

	//tar && gzip
	// file write
	os.MkdirAll(filepath.Dir(fname), 777)
	fw, err := os.Create(fname)
	if err != nil {
		return "", err
	}
	defer fw.Close()

	// gzip write
	gw := gzip.NewWriter(fw)
	defer gw.Close()

	// tar write
	tw := tar.NewWriter(gw)
	defer tw.Close()

	for k, v := range diff {
		fpath := path.Join(rootdir, k)
		h := new(tar.Header)
		h.Name = k
		h.Size = v.NewSize
		h.Mode = int64(v.Mode)
		h.ModTime = v.ModTime

		err = tw.WriteHeader(h)
		if err != nil {
			return "", err
		}
		fr, err := os.Open(fpath)
		if err != nil {
			return "", err
		}
		_, err = io.Copy(tw, fr)
		if err != nil {
			return "", err
		}
	}

	return fname, nil
}
