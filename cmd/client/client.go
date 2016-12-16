package main

import (
	"bufio"
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
	"time"
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

var config SyncConfig
var host, dir, app string
var checkUpdate bool

func main() {
	defer func() {
		reader := bufio.NewReader(os.Stdin)
		fmt.Println("press any key to exit")
		_, _ = reader.ReadByte()
	}()

	flag.BoolVar(&config.SyncDetail, "v", false, "show detail info")
	flag.StringVar(&host, "host", "", "sync server")
	flag.StringVar(&dir, "dir", "", "sync dir")
	flag.StringVar(&app, "app", "", "sync app")
	flag.BoolVar(&checkUpdate, "check", false, "check update")
	flag.Parse()

	wd := filepath.Dir(os.Args[0])
	autoupdate := filepath.Join(wd, ".autoupdate")
	content, err := ioutil.ReadFile(autoupdate)
	if err == nil {
		json.Unmarshal(content, &config)
	}

	//overwrite with command line arguments
	if len(host) > 0 {
		config.SyncHost = host
	}
	if len(dir) > 0 {
		config.SyncDir = dir
	}
	if len(config.SyncDir) == 0 {
		config.SyncDir = wd
	}
	if len(app) > 0 {
		config.SyncApp = app
	}
	if len(config.SyncApp) == 0 {
		config.SyncApp = filepath.Base(wd)
	}

	if len(config.SyncHost) == 0 {
		usage()
		return
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
			log.Printf("has update:%s", gresp.PatchFile)
			return
		}
	}

	//get diff file
	requrl = fmt.Sprintf("http://%s%s", config.SyncHost, gresp.PatchFile)
	if config.SyncDetail {
		log.Printf("downloading %s\n", requrl)
	}
	resp, err = http.Get(requrl)
	if err != nil {
		log.Fatal(err)
	}

	updates, err := gsync.ApplyDiff(config.SyncDir, resp.Body, gresp.Diff, config.Ignore)
	if err != nil {
		log.Fatal(err)
	}
	resp.Body.Close()
	if len(updates) > 0 {
		if config.SyncDetail {
			for _, u := range updates {
				log.Printf("%s updated\r\n", u)
			}
		}
		log.Printf("update successfully\n")
		//set access time and modify time to mark updated
		os.Chtimes(autoupdate, time.Now(), time.Now())
	} else {
		log.Printf("up to date\n")
	}
}
