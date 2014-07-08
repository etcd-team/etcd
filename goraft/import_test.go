package goraft

import (
	"reflect"
	"testing"
)

func TestReadConfFile(t *testing.T) {
	c, err := ReadConfFile("fixtures/1.local.etcd/conf")
	if err != nil {
		t.Fatalf("read conf file error: %v", err)
	}

	w := &Config{CommitIndex: 13, Peers: []*Peer{{"machine2", ""}, {"machine3", ""}}}
	if !reflect.DeepEqual(c, w) {
		t.Errorf("conf = %+v, want %+v", c, w)
	}
}

func TestReadLogFile(t *testing.T) {
	entries, err := ReadLogFile("fixtures/1.local.etcd/log")
	if err != nil {
		t.Fatalf("read log file error: %v", err)
	}

	w := 610
	if g := len(entries); g != w {
		t.Errorf("len(entries) = %d, want %d", g, w)
	}
	startIndex := uint64(11838)
	for _, e := range entries {
		if g := e.GetIndex(); g != startIndex {
			t.Errorf("index = %d, want %d", g, startIndex)
		}
		startIndex++
	}
}

func TestGetLatestSnapshotName(t *testing.T) {
	name, err := GetLatestSnapshotName("fixtures/1.local.etcd/snapshot")
	if err != nil {
		t.Fatalf("get latest snapshot name error: %v", err)
	}

	w := "5_12048.ss"
	if g := name; g != w {
		t.Errorf("name = %s, want %s", g, w)
	}
}

func TestReadSnapshotFile(t *testing.T) {
	snapshot, err := ReadSnapshotFile("fixtures/1.local.etcd/snapshot/5_12048.ss")
	if err != nil {
		t.Fatalf("read snapshot file error: %v", err)
	}

	windex := uint64(12048)
	if g := snapshot.LastIndex; g != windex {
		t.Errorf("lastIndex = %d, want %d", g, windex)
	}
	wterm := uint64(5)
	if g := snapshot.LastTerm; g != wterm {
		t.Errorf("lastTerm = %d, want %d", g, wterm)
	}
	wpeers := []*Peer{{"1.local", ""}}
	if g := snapshot.Peers; !reflect.DeepEqual(g, wpeers) {
		t.Errorf("peers = %+v, want %+v", g, wpeers)
	}
	wpath := "1.local.etcd/snapshot/5_12048.ss"
	if g := snapshot.Path; g != wpath {
		t.Errorf("path = %s, want %s", g, wpath)
	}
}

func TestLoadDataSet(t *testing.T) {
	dataset, err := LoadDataset("fixtures/1.local.etcd")
	if err != nil {
		t.Fatalf("load dataset error: %v", err)
	}

	if dataset.conf == nil {
		t.Errorf("conf = nil, want sth")
	}
	if dataset.entries == nil {
		t.Errorf("entries = nil, want sth")
	}
	if dataset.snapshot == nil {
		t.Errorf("snapshot = nil, want sth")
	}
}
