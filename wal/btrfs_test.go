package wal

import (
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestSetNOCOW(t *testing.T) {
	f, err := ioutil.TempFile(".", "etcdtest")
	if err != nil {
		t.Fatal("Failed creating temp dir")
	}
	name := f.Name()
	defer func() {
		f.Close()
		os.Remove(name)
	}()

	if IsBtrfs(name) {
		SetNOCOWFile(name)
		out, err := exec.Command("lsattr", name).Output()
		if err != nil {
			t.Fatal("Failed executing lsattr")
		}
		if !strings.Contains(string(out), "---------------C") {
			t.Fatal("Failed setting NOCOW:\n", string(out))
		}
	}
}
