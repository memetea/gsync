package main

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
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
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/julienschmidt/httprouter"
)

type Config struct {
	Listen   string
	CacheDir string
	Apps     map[string]*struct {
		AppDir string `json:"dir"`
	}
}

var (
	wd         string
	config     Config
	appCache   *gsync.HotCache
	appCacheMu sync.Mutex

	fileHashMap   map[string]string
	fileHashMapMu sync.Mutex
)

func watchApps(apps []string, stopc <-chan struct{}, eventc chan fsnotify.Event) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()
	go func() {
		for {
			select {
			case event := <-watcher.Events:
				//log.Printf("event2: %s", event)
				eventc <- event
			case err := <-watcher.Errors:
				log.Println("error:", err)
			}
		}
	}()

	for _, appFolder := range apps {
		err = watcher.Add(appFolder)
		if err != nil {
			log.Fatal(err)
		}
		filepath.Walk(appFolder, func(path string, info os.FileInfo, err error) error {
			if info.IsDir() {
				//log.Printf("watch dir:%s", path)
				watcher.Add(path)
			}
			return nil
		})
	}
	<-stopc
}

func watchFileEvents(c <-chan fsnotify.Event) {
	for {
		select {
		case event, ok := <-c:
			if ok {
				log.Printf("event: %s", event)
				if (event.Op & fsnotify.Write) == fsnotify.Write {
					if _, ok := appCache.Get(event.Name); ok {
						//cal hash and cache new file content if file was written
						hashAndCacheFile(event.Name)
					}
				}

				if (event.Op&fsnotify.Rename) == fsnotify.Rename || (event.Op&fsnotify.Remove) == fsnotify.Remove {
					//delete file cache if it was renamed or removed
					appCache.Delete(event.Name)
					delete(fileHashMap, event.Name)
				}
			}
		}
	}
}

func readConfig() {
	//read config
	content, err := ioutil.ReadFile(filepath.Join(wd, "config.txt"))
	if err != nil {
		log.Fatal(err)
	}

	err = json.Unmarshal(content, &config)
	if err != nil {
		log.Fatal(err)
	}
}

func hashAndCacheFile(fp string) error {
	fp = filepath.Clean(fp)
	fr, err := os.Open(fp)
	if err != nil {
		return err
	}
	content, err := ioutil.ReadAll(fr)
	if err != nil {
		return err
	}
	fr.Close()
	fileHash := fmt.Sprintf("%x", md5.Sum(content))
	fileHashMapMu.Lock()
	fileHashMap[fp] = fileHash
	log.Printf("file %s hash:%s", fp, fileHash)
	fileHashMapMu.Unlock()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, gw.Name = filepath.Split(fp)
	fi, err := os.Stat(fp)
	if err != nil {
		return err
	}
	gw.ModTime = fi.ModTime()

	_, err = io.Copy(gw, bytes.NewReader(content))
	if err != nil {
		return err
	}
	gw.Close()

	content, _ = ioutil.ReadAll(&buf)
	appCache.AddItem(fp, content, 24*time.Hour)
	return nil
}

func createHttpRouter() http.Handler {
	router := httprouter.New()

	router.GET("/app/:app/*file", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		appName := p.ByName("app")
		file := p.ByName("file")
		app, ok := config.Apps[appName]
		if !ok {
			http.Error(w, "file not found", 404)
			return
		}

		fp := filepath.Clean(filepath.Join(app.AppDir, file))
		content, ok := appCache.Get(fp)
		if !ok {
			err := hashAndCacheFile(fp)
			if err != nil {
				http.Error(w, "error cache file", 404)
				return
			}
			if content, ok = appCache.Get(fp); !ok {
				http.Error(w, "not found", 404)
				return
			}
		}
		w.Header().Set("Content-Encoding", "gzip")
		io.Copy(w, bytes.NewReader(content.([]byte)))
	})

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
		err := json.Unmarshal([]byte(r.FormValue("req")), req)
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
			if req.ClientVersion == 0 {
				resp.PatchFile, err = gsync.PrepareDiff(app.AppDir, config.CacheDir, diff)
				if err != nil {
					http.Error(w, fmt.Sprintf("prepare diff error:%s", err), 500)
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
		}
		resp.Diff = diff
		content, err := json.Marshal(resp)
		if err != nil {
			http.Error(w, "marshal response error", 500)
			return
		}
		fmt.Fprintf(w, string(content))
	})
	return router
}

func main() {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	os.Chdir(dir)
	if err != nil {
		log.Fatal(err)
	}
	wd = dir

	//read config
	readConfig()
	if len(config.Apps) == 0 {
		log.Printf("none apps were configed\n")
		return
	}

	appCache = gsync.CreateCache(100)
	fileHashMap = make(map[string]string)

	//watch file modify events
	in, out := NotifyPipeChan(500 * time.Millisecond)
	stopc := make(chan struct{})
	var apps []string
	for _, app := range config.Apps {
		if !filepath.IsAbs(app.AppDir) {
			app.AppDir = filepath.Join(wd, app.AppDir)
		}
		if _, err := os.Stat(app.AppDir); err != nil && !os.IsExist(err) {
			continue
		}

		log.Printf("watch dir:%s", app.AppDir)
		apps = append(apps, app.AppDir)
	}
	go watchApps(apps, stopc, in)
	go watchFileEvents(out)

	router := createHttpRouter()
	log.Printf("listen on %s", config.Listen)
	log.Fatal(http.ListenAndServe(config.Listen, router))
}
