package main

import (
	"bufio"
	"crypto/md5"
	"encoding/json"
	"flag"
	"fmt"
	"gsync"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

var (
	wd          string
	config      SyncConfig
	checkUpdate bool
)

type SyncConfig struct {
	SyncDetail  bool `json:"-"`
	SyncCounter int  `json:"-"`
	SyncHost    string
	SyncDir     string `json:"-"`
	SyncApp     string
	Ignore      []string
}

func usage() {
	fileName := filepath.Base(os.Args[0])
	fmt.Printf("usage:\n%s --host=[host] --dir=[dir] --app=[app]\n", fileName)
}

func cleanTmpFiles(dir string) error {
	fis, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Fatal(err)
	}
	for _, fi := range fis {
		fname := fi.Name()
		if !fi.IsDir() {
			if path.Ext(fname) == ".autoupdatetmpfile" {
				err = os.Remove(path.Join(dir, fname))
				if err != nil {
					return err
				}
			}
		} else if fname != "." && fname != ".." {
			err = cleanTmpFiles(path.Join(dir, fname))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func main() {
	wd = filepath.Dir(os.Args[0])

	defer func() {
		reader := bufio.NewReader(os.Stdin)
		fmt.Println("press any key to exit")
		_, _ = reader.ReadByte()
	}()

	flag.BoolVar(&config.SyncDetail, "v", false, "show detail info")
	flag.StringVar(&config.SyncHost, "host", "", "sync server")
	flag.StringVar(&config.SyncDir, "dir", "", "sync dir")
	flag.StringVar(&config.SyncApp, "app", "", "sync app")
	flag.BoolVar(&checkUpdate, "check", false, "check update")
	flag.Parse()

	autoupdate := filepath.Join(wd, ".autoupdate")
	content, err := ioutil.ReadFile(autoupdate)
	if err == nil {
		json.Unmarshal(content, &config)
	}

	if len(config.SyncHost) == 0 {
		usage()
		return
	}
	if len(config.SyncDir) == 0 {
		config.SyncDir = wd
	}
	if config.SyncDetail {
		log.Printf("sync app(%s) from %s\n", config.SyncApp, config.SyncHost)
	}

	//clean autoupdatetmpfiles
	err = cleanTmpFiles(config.SyncDir)
	if err != nil {
		log.Println(err)
	}

	req, err := gsync.MakeRequest(config.SyncDir, config.Ignore, true)
	if err != nil {
		log.Fatal(err)
	}
	req.ClientVersion = 2

	//check update
	content, err = json.Marshal(req)
	if err != nil {
		log.Fatal(err)
	}

	requrl := fmt.Sprintf("http://%s/hasupdate/%s", config.SyncHost, config.SyncApp)
	if config.SyncDetail {
		log.Printf("request %s. param:%s\n", requrl, content)
	}
	resp, err := http.PostForm(requrl, url.Values{"req": {string(content)}})
	if err != nil {
		log.Fatal(err)
	}

	content, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	if config.SyncDetail {
		log.Printf("response:%s\n", content)
	}

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("check update error:%s", content)
	}
	resp.Body.Close()

	var gresp gsync.Response
	err = json.Unmarshal(content, &gresp)
	if err != nil {
		log.Fatal(err)
	}

	if len(gresp.Diff) == 0 {
		//no update
		log.Printf("up to date\n")
		return
	} else {
		if checkUpdate {
			var totalSize int64
			for _, d := range gresp.Diff {
				totalSize += d.NewSize
			}
			log.Printf("has update. files:%d, size:%d", len(gresp.Diff), totalSize)
			return
		}
	}

	//download files
	var upCount int32
	if len(gresp.Diff) > 0 {
		var wg sync.WaitGroup
		for fname, diff := range gresp.Diff {
			wg.Add(1)
			go func(fname string, d gsync.Diff) {
				defer wg.Done()
				//get diff file
				requrl := fmt.Sprintf("http://%s/app/%s/%s", config.SyncHost, config.SyncApp, fname)
				if config.SyncDetail {
					log.Printf("downloading %s\n", requrl)
				}
				resp, err := http.Get(requrl)
				if err != nil {
					log.Printf("get url error:%s", err)
					return
				}

				fileContent, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					log.Printf("read response error:%v", err)
					return
				}

				if resp.StatusCode != http.StatusOK {
					log.Printf("download %s failed: statusCode=%d, response=%s",
						fname, resp.StatusCode, fileContent)
					return
				}

				newHash := fmt.Sprintf("%x", md5.Sum(fileContent))
				if newHash != d.NewHash {
					log.Printf("file %s hash check failed. expect %s, got %s", fname, d.NewHash, newHash)
					return
				}
				err = gsync.ReplaceFile(fileContent, filepath.Join(wd, fname), d.Mode, d.ModTime)
				if err != nil {
					log.Printf("replace file error:%v", err)
					return
				}
				atomic.AddInt32(&upCount, 1)
			}(fname, diff)
		}

		wg.Wait()

		if int(upCount) == len(gresp.Diff) {
			os.Chtimes(autoupdate, time.Now(), time.Now())
			log.Printf("update successfully\n")
		} else {
			log.Printf("update failed. %d of %d was updated\n", upCount, len(gresp.Diff))
		}
	} else {
		log.Printf("up to date\n")
	}
}
