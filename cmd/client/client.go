package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"gsync"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

type SyncConfig struct {
	SyncDetail bool `json:"-"`
	SyncHost   string
	SyncDir    string `json:"-"`
	SyncApp    string
	Ignore     []string
	LastUpdate time.Time
}

func usage() {
	fmt.Printf("usage:\n\tclient --host=[host] --dir=[dir] --app=[app]\n")
}

var config SyncConfig

func main() {
	flag.BoolVar(&config.SyncDetail, "v", false, "show detail info")
	flag.StringVar(&config.SyncHost, "host", "", "sync server")
	flag.StringVar(&config.SyncHost, "h", "", "sync server")
	flag.StringVar(&config.SyncDir, "dir", "", "sync dir")
	flag.StringVar(&config.SyncDir, "d", "", "sync dir")
	flag.StringVar(&config.SyncApp, "app", "", "sync app")
	flag.StringVar(&config.SyncApp, "a", "", "sync app")
	flag.Parse()

	wd, _ := os.Getwd()
	var cnf SyncConfig
	autoupdate := filepath.Join(wd, ".autoupdate")
	content, err := ioutil.ReadFile(autoupdate)
	if err == nil {
		json.Unmarshal(content, &config)
	}

	if len(config.SyncHost) == 0 {
		//try parse from .autoupdate
		config.SyncHost = cnf.SyncHost
	}
	if len(config.SyncDir) == 0 {
		config.SyncDir = wd
	}
	if len(config.SyncApp) == 0 {
		if len(cnf.SyncApp) > 0 {
			config.SyncApp = cnf.SyncApp
		} else {
			config.SyncApp = filepath.Base(wd)
		}
	}

	if len(config.SyncHost) == 0 {
		usage()
		return
	}
	if config.SyncDetail {
		log.Printf("%s\n", content)
	}
	req, err := gsync.MakeRequest(config.SyncDir, true)
	if err != nil {
		log.Fatal(err)
	}
	gsync.FilterIgnore(req, config.Ignore)

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
		log.Printf("update to date\n")
		goto L
	}

	//get diff file
	requrl = fmt.Sprintf("http://%s%s", config.SyncHost, gresp.PatchFile)
	if config.SyncDetail {
		log.Printf("request %s\n", requrl)
	}
	resp, err = http.Get(requrl)
	if err != nil {
		log.Fatal(err)
	}

	err = gsync.ApplyDiff(config.SyncDir, resp.Body, gresp.Diff)
	if err != nil {
		log.Fatal(err)
	}
	resp.Body.Close()
	config.LastUpdate = time.Now()
	log.Printf("update success.")

L:
	content, err = json.Marshal(config)
	if err == nil {
		gsync.UnHideFile(autoupdate)
		err = ioutil.WriteFile(autoupdate, content, 0777)
		if err != nil {
			log.Fatal(err)
		}
		err = gsync.HideFile(autoupdate)
		if err != nil {
			log.Fatal(err)
		}
	}
}
