package goraft

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"sort"

	"github.com/coreos/etcd/goraft/protobuf"
)

type Dataset struct {
	Config   *Config
	Snapshot *Snapshot
	Entries  []*protobuf.LogEntry
}

// A peer is a reference to another server involved in the consensus protocol.
type Peer struct {
	Name             string `json:"name"`
	ConnectionString string `json:"connectionString"`
}

// Snapshot represents an in-memory representation of the current state.
type Snapshot struct {
	LastIndex uint64 `json:"lastIndex"`
	LastTerm  uint64 `json:"lastTerm"`

	// cluster configuration
	Peers []*Peer `json:"peers"`
	// data of state machine
	State []byte `json:"state"`
	Path  string `json:"path"`
}

type Config struct {
	// last commit index on config changes
	CommitIndex uint64  `json:"commitIndex"`
	Peers       []*Peer `json:"peers"`
}

func LoadDataset(dirpath string) (*Dataset, error) {
	conf, err := ReadConfFile(path.Join(dirpath, "conf"))
	if err != nil {
		return nil, err
	}

	entries, err := ReadLogFile(path.Join(dirpath, "log"))
	if os.IsNotExist(err) {
		entries = make([]*protobuf.LogEntry, 0)
	} else if err != nil {
		return nil, err
	}

	snapshotName, err := GetLatestSnapshotName(path.Join(dirpath, "snapshot"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	var snapshot *Snapshot
	if snapshotName != "" {
		snapshot, err = ReadSnapshotFile(path.Join(dirpath, "snapshot", snapshotName))
		if err != nil {
			return nil, err
		}
	}

	return &Dataset{Config: conf, Entries: entries, Snapshot: snapshot}, nil
}

func ReadConfFile(path string) (*Config, error) {
	// open conf file
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	conf := &Config{}
	if err = json.Unmarshal(b, conf); err != nil {
		return nil, err
	}

	return conf, nil
}

// ReadLogFile reads log entries from log file.
// Entries are not guaranteed to be after startIndex.
func ReadLogFile(path string) ([]*protobuf.LogEntry, error) {
	file, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return DecodeLogEntries(file)
}

// GetLatestSnapshotName gets the latest snapshot from the directory.
func GetLatestSnapshotName(path string) (string, error) {
	// Open snapshot/ directory.
	dir, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return "", err
	}
	defer dir.Close()

	// Retrieve a list of all snapshots.
	filenames, err := dir.Readdirnames(-1)
	if err != nil {
		return "", err
	}
	if len(filenames) == 0 {
		return "", nil
	}

	// Grab the latest snapshot.
	sort.Strings(filenames)
	return filenames[len(filenames)-1], nil
}

// ReadSnapshotFile reads snapshot file.
func ReadSnapshotFile(path string) (*Snapshot, error) {
	// Read snapshot data.
	file, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Check checksum.
	var checksum uint32
	n, err := fmt.Fscanf(file, "%08x\n", &checksum)
	if err != nil {
		return nil, err
	} else if n != 1 {
		return nil, errors.New("miss heading checksum")
	}

	// Load remaining snapshot contents.
	b, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}

	// Generate checksum.
	byteChecksum := crc32.ChecksumIEEE(b)
	if uint32(checksum) != byteChecksum {
		return nil, errors.New("bad checksum")
	}

	// Decode snapshot.
	snapshot := new(Snapshot)
	if err = json.Unmarshal(b, snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func DecodeLogEntries(file *os.File) ([]*protobuf.LogEntry, error) {
	readBytes := int64(0)
	entries := make([]*protobuf.LogEntry, 0)

	// Read the file and decode entries.
	for {
		entry, n, err := DecodeLogEntry(file)
		if err != nil {
			if err != io.EOF {
				log.Println("only succeed to decode part of log file:", err)
			}
			break
		}

		// Append entry.
		entries = append(entries, entry)
		readBytes += int64(n)
	}

	return entries, nil
}

// Decodes the log entry from a buffer. Returns the number of bytes read and
// any error that occurs.
func DecodeLogEntry(r io.Reader) (*protobuf.LogEntry, int, error) {
	var length int
	_, err := fmt.Fscanf(r, "%8x\n", &length)
	if err != nil {
		return nil, -1, err
	}

	data := make([]byte, length)
	_, err = io.ReadFull(r, data)
	if err != nil {
		return nil, -1, err
	}

	le := new(protobuf.LogEntry)
	if err = le.Unmarshal(data); err != nil {
		return nil, -1, err
	}

	return le, length + 8 + 1, nil
}
