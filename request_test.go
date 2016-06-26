package gsync

import (
	"encoding/json"
	"os"
	"path"
	"testing"
)

func Test_MakeRequest(t *testing.T) {
	wd, _ := os.Getwd()

	req, err := MakeRequest(path.Join(wd, "testdata/old"), true)
	if err != nil {
		t.Fatal(err)
	}
	content, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s", content)
}

func Test_CalcDiff(t *testing.T) {
	wd, _ := os.Getwd()
	req, err := MakeRequest(path.Join(wd, "testdata/old"), true)
	if err != nil {
		t.Fatal(err)
	}

	diffMap, err := CalcDiff(path.Join(wd, "testdata/new"), req)
	if err != nil {
		t.Fatal(err)
	}
	content, err := json.Marshal(diffMap)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s", content)

	fpath, err := PrepareDiff(path.Join(wd, "testdata/new"), diffMap)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(fpath)
}
