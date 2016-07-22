package gsync

import (
	"archive/tar"
	"compress/gzip"
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

//Diff Describes a difference between two files
type Diff struct {
	NewHash string
	OldHash string
	NewSize int64
	Mode    os.FileMode
	ModTime time.Time
}

//DiffMap Holds differences
type DiffMap map[string]Diff

type Response struct {
	PatchFile string
	PatchSize int64
	Diff      DiffMap
}

func calcDiff(stripdir, curdir string, req *Request, diffMap DiffMap) error {
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

//CalcDiff calcs diffrence on req with cmpdir
func CalcDiff(cmpdir string, req *Request) (DiffMap, error) {
	diffMap := make(DiffMap)
	cmpdir = normpath(path.Clean(cmpdir))
	if !strings.HasSuffix(cmpdir, "/") {
		cmpdir = cmpdir + "/"
	}
	err := calcDiff(cmpdir, cmpdir, req, diffMap)
	return diffMap, err
}

func CalcDiffOnFolders(cmpfrom string, cmpto string) (DiffMap, error) {
	//calc request on cmpfrom
	req, err := MakeRequest(cmpto, true)
	if err != nil {
		return nil, err
	}
	return CalcDiff(cmpfrom, req)
}

//PrepareDiff
func PrepareDiff(rootdir string, cachedir string, diff DiffMap) (string, error) {
	//concat hash
	d := md5.New()
	for _, v := range diff {
		d.Write([]byte(v.NewHash))
	}
	fname := fmt.Sprintf("%x", d.Sum(nil))
	fname = path.Join(cachedir, fmt.Sprintf("%s.tar.gz", fname))
	_, err := os.Stat(fname)
	if err == nil || os.IsExist(err) {
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
		fmt.Printf("write %s size:%d\n", k, v.NewSize)
		fpath := path.Join(rootdir, k)
		h := new(tar.Header)
		h.Name = k
		h.Size = v.NewSize
		h.Mode = int64(v.Mode)
		h.ModTime = v.ModTime
		fmt.Printf("write %s\n", fpath)
		err = tw.WriteHeader(h)
		if err != nil {
			return "", err
		}
		fr, err := os.Open(fpath)
		if err != nil {
			return "", err
		}
		n, err := io.Copy(tw, fr)
		if err != nil {
			fmt.Printf("write bytes:%d, err %v\n", n, err)
			return "", err
		}
	}

	return fname, nil
}

func ApplyDiff(applydir string, df io.Reader, diff DiffMap, ignore []string) (int, error) {
	// gzip reader
	gr, err := gzip.NewReader(df)
	if err != nil {
		return 0, err
	}
	defer gr.Close()
	// tar reader
	tr := tar.NewReader(gr)
	//clean path
	for i, s := range ignore {
		ignore[i] = strings.Replace(filepath.Clean(s), "\\", "/", -1)
	}
	counter := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// end of tar archive
			break
		}
		if err != nil {
			return 0, err
		}

		tofile := filepath.Join(applydir, hdr.Name)
		os.MkdirAll(filepath.Dir(tofile), 0777)
		//check ignore
		skip := false
		for _, ignoreFile := range ignore {
			if match, err := filepath.Match(ignoreFile, hdr.Name); err == nil && match {
				skip = true
				break
			}
		}
		if skip {
			//do not overwrite existing file marked as ignore
			if _, err = os.Stat(tofile); err == nil {
				continue
			}
		}

		fw, err := os.OpenFile(tofile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0)
		if err != nil {
			//try rename file and reopen
			err = os.Rename(tofile, tofile+".autoupdatetmpfile")
			if err != nil {
				return 0, err
			}
			fw, err = os.OpenFile(tofile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0)
			if err != nil {
				os.Rename(tofile+".autoupdatetmpfile", tofile)
				return 0, err
			}

			HideFile(tofile + ".autoupdatetmpfile")
		}

		log.Printf("overwrite:%s\n", tofile)
		if _, err := io.Copy(fw, tr); err != nil {
			return 0, err
		}
		fw.Close()
		if di, ok := diff[hdr.Name]; ok {
			os.Chmod(tofile, di.Mode)
			os.Chtimes(tofile, di.ModTime, di.ModTime)
		}
		counter++
	}
	return counter, nil
}
