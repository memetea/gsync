package main

import (
	"encoding/json"
	"fmt"
	"gsync"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/julienschmidt/httprouter"
)

type Config struct {
	Listen   string
	CacheDir string
	Apps     map[string]struct {
		AppDir string `json:"dir"`
	}
}

var config Config

func main() {
	//read config
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	os.Chdir(dir)
	if err != nil {
		log.Fatal(err)
	}
	content, err := ioutil.ReadFile(filepath.Join(dir, "config.txt"))
	if err != nil {
		log.Fatal(err)
	}

	err = json.Unmarshal(content, &config)
	if err != nil {
		log.Fatal(err)
	}

	if len(config.Apps) == 0 {
		log.Printf("none apps to sync\n")
		return
	}

	router := httprouter.New()

	router.GET("/tmpfiles/:file", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		file := p.ByName("file")
		fr, err := os.Open(filepath.Join(config.CacheDir, file))
		if err != nil {
			http.Error(w, "file not found", 404)
		}
		defer fr.Close()
		io.Copy(w, fr)
	})

	router.POST("/hasupdate/:app", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		resp := &gsync.Response{}
		app, ok := config.Apps[p.ByName("app")]
		if !ok {
			content, _ := json.Marshal(resp)
			fmt.Fprintf(w, string(content))
			return
		}
		r.ParseForm()
		req := &gsync.Request{}
		err = json.Unmarshal([]byte(r.FormValue("req")), req)
		if err != nil {
			http.Error(w, "invalid request", 500)
			return
		}
		diff, err := gsync.CalcDiff(app.AppDir, req)
		if err != nil {
			http.Error(w, "calc diff error", 500)
			return
		}
		if len(diff) != 0 {
			resp.PatchFile, err = gsync.PrepareDiff(app.AppDir, config.CacheDir, diff)
			if err != nil {
				http.Error(w, "prepare diff error", 500)
				return
			}
			fi, err := os.Stat(resp.PatchFile)
			if err != nil {
				http.Error(w, "query patch size error", 500)
				return
			}
			resp.PatchSize = fi.Size()
			resp.PatchFile = strings.Replace(resp.PatchFile, config.CacheDir, "/tmpfiles", 1)
		}
		resp.Diff = diff
		content, err = json.Marshal(resp)
		if err != nil {
			http.Error(w, "marshal response error", 500)
			return
		}
		fmt.Fprintf(w, string(content))
	})

	log.Printf("listen on %s", config.Listen)
	log.Fatal(http.ListenAndServe(config.Listen, router))
}
