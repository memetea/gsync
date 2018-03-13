package gsync

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/djherbis/times"
)

func Test_CopyFile(t *testing.T) {
	tmpdir, err := ioutil.TempDir(os.TempDir(), "gsync.")
	if err != nil {
		t.Fatal(err)
	}
	fpath := filepath.Join(tmpdir, "1.txt")
	f1, err := os.Create(fpath)
	if err != nil {
		t.Logf("create test file %s failed", fpath)
	}
	f1.WriteString("test content")
	f1.Close()

	time.Sleep(5 * time.Second)
	fto := filepath.Join(tmpdir, "2.txt")
	err = copyFile(fpath, fto, true)
	if err != nil {
		t.Fatal(err)
	}

	//check contents
	content1, err := ioutil.ReadFile(fpath)
	if err != nil {
		t.Fatal(err)
	}
	content2, err := ioutil.ReadFile(fto)
	if err != nil {
		t.Fatal(err)
	}
	if md5.Sum(content1) != md5.Sum(content2) {
		t.Fatal("copy err: content not equal")
	}

	//check atime, ctime
	fi1, err := os.Stat(fpath)
	if err != nil {
		t.Fatal(err)
	}
	fi2, err := os.Stat(fto)
	if err != nil {
		t.Fatal(err)
	}
	if times.Get(fi1).AccessTime() != times.Get(fi2).AccessTime() {
		t.Fatalf("access time not equal")
	}
	if times.Get(fi1).ModTime() != times.Get(fi2).ModTime() {
		t.Fatalf("mod time not equal")
	}
	os.RemoveAll(tmpdir)
}

func Test_CopyDir(t *testing.T) {
	tmpdir1, err := ioutil.TempDir(os.TempDir(), "gsync.")
	if err != nil {
		t.Fatal(err)
	}

	tmpdir2, err := ioutil.TempDir(os.TempDir(), "gsync.")
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(filepath.Join(tmpdir1, "1.txt"))
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("hello world")
	f.Close()

	f, err = os.Create(filepath.Join(tmpdir1, "2.txt"))
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("hello world2")
	f.Close()

	fpath := filepath.Join(tmpdir1, "subfold/3.txt")
	os.MkdirAll(filepath.Dir(fpath), 0777)
	f, err = os.Create(fpath)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("hello world3")
	f.Close()

	err = copyDir(tmpdir1, tmpdir2, true)
	if err != nil {
		t.Fatal(err)
	}

	diff, err := CalcDiffOnFolders(tmpdir1, tmpdir2)
	if err != nil {
		t.Fatal(err)
	}

	if len(diff) != 0 {
		t.Fatalf("directory not equal: %#v", diff)
	}

	os.RemoveAll(tmpdir1)
	os.RemoveAll(tmpdir2)
}

func Test_MakeRequest(t *testing.T) {
	wd, _ := os.Getwd()

	// match, err := filepath.Match("shouldbeignore/*", "shouldbeignore/subdir/1.txt")
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// if !match {
	// 	t.Fatal("expect match")
	// }

	ignore := []string{"shouldbeignore/*"}

	req, err := MakeRequest(path.Join(wd, "testdata/old"), ignore, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Hashes) != 4 {
		for k := range req.Hashes {
			t.Logf("key:%s", k)
		}
		t.Fatalf("expect 4 file info. got %d", len(req.Hashes))
	}
	content, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s", content)
}

func Test_ApplyDiff(t *testing.T) {
	newdir, err := ioutil.TempDir(os.TempDir(), "gsync.")
	if err != nil {
		t.Fatal(err)
	}

	olddir, err := ioutil.TempDir(os.TempDir(), "gsync.")
	if err != nil {
		t.Fatal(err)
	}

	cachedir, err := ioutil.TempDir(os.TempDir(), "gsync.")
	if err != nil {
		t.Fatal(err)
	}

	wd, _ := os.Getwd()
	if err = copyDir(filepath.Join(wd, "testdata/new"), newdir, true); err != nil {
		t.Fatal(err)
	}
	if err = copyDir(filepath.Join(wd, "testdata/old"), olddir, true); err != nil {
		t.Fatal(err)
	}

	diffMap, err := CalcDiffOnFolders(newdir, olddir)
	if err != nil {
		t.Fatal(err)
	}
	diffFile, err := PrepareDiff(newdir, cachedir, diffMap)
	if err != nil {
		t.Fatal(err)
	}

	df, err := os.Open(diffFile)
	if err != nil {
		t.Fatal(err)
	}
	defer df.Close()
	_, err = ApplyDiff(olddir, df, diffMap, nil)
	if err != nil {
		t.Fatal(err)
	}

	diffMap, err = CalcDiffOnFolders(newdir, olddir)
	if err != nil {
		t.Fatal(err)
	}
	if len(diffMap) != 0 {
		t.Fatalf("apply failed. there're differences:%#v", diffMap)
	}
	os.RemoveAll(newdir)
	os.RemoveAll(olddir)
	os.RemoveAll(cachedir)
}

func Test_FilterIgnore(t *testing.T) {
	req := &Request{
		Hashes: make(map[string]string),
	}
	req.Hashes["client.exe"] = "123452233"
	req.Hashes[".autoupdate"] = "321324323131..."
	req.Hashes["config.txt"] = "hellll"
	req.Hashes["logs/1.log"] = "log1 content"
	req.Hashes["logs/2.log"] = "log2 content"
	req.Hashes["app.log"] = "app log content"

	ignore := []string{"client.exe", ".autoupdate", "logs/*.log"}
	FilterIgnore(req, ignore)

	if _, ok := req.Hashes["client.exe"]; ok {
		t.Fatal("client.exe should be filter out")
	}

	if _, ok := req.Hashes["config.txt"]; !ok {
		t.Fatal("config.txt should not be filtered out")
	}

	if len(req.Hashes) != 2 {
		t.Fatal("unexpected filter result")
	}

	t.Logf("%v", req.Hashes)
}

func TestHotCache(t *testing.T) {
	cache := CreateCache(10)
	value := []byte{0, 1, 2, 3, 4, 5}
	cache.AddItem("file1", value, 24*time.Hour)
	value2, ok := cache.GetBytes("file1")
	if !ok {
		t.Fatal("get from cache failed")
	}
	if !bytes.Equal(value2, value) {
		t.Fatal("cache not match")
	}
}

func TestDebounceChan(t *testing.T) {
	in, out := DebouncePipeChan(2 * time.Second)
	go func() {
		for i := 0; i < 10; i++ {
			in <- i
			log.Printf("input %d", i)
			time.Sleep(1 * time.Second)
		}
		close(in)
	}()

	var sum int
	for {
		v, ok := <-out
		if !ok {
			break
		}
		log.Printf("get %v", v)
		sum += v.(int)
	}

	log.Printf("sum=%d", sum)
}
