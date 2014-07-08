package goraft

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

// Creates a new instance of a command by name.
func NewCommand(name string, data []byte) (interface{}, error) {
	var cmd interface{}

	switch name {
	case "etcd:remove":
		cmd = &RemoveCommand{}
	case "etcd:join":
		cmd = &JoinCommand{}
	case "etcd:setClusterConfig":
		cmd = &SetClusterConfigCommand{}
	case "etcd:compareAndDelete":
		cmd = &CompareAndDeleteCommand{}
	case "etcd:compareAndSwap":
		cmd = &CompareAndSwapCommand{}
	case "etcd:create":
		cmd = &CreateCommand{}
	case "etcd:delete":
		cmd = &DeleteCommand{}
	case "etcd:set":
		cmd = &SetCommand{}
	case "etcd:sync":
		cmd = &SyncCommand{}
	case "etcd:update":
		cmd = &UpdateCommand{}
	case "raft:join":
		cmd = &DefaultJoinCommand{}
	case "raft:leave":
		cmd = &DefaultLeaveCommand{}
	case "raft:nop":
		cmd = &NOPCommand{}
	default:
		return nil, fmt.Errorf("unregistered command type %s", name)
	}

	// If data for the command was passed in the decode it.
	if data != nil {
		if err := json.NewDecoder(bytes.NewReader(data)).Decode(cmd); err != nil {
			return nil, err
		}
	}
	return cmd, nil
}

// The RemoveCommand removes a server from the cluster.
type RemoveCommand struct {
	Name string `json:"name"`
}

// JoinCommand represents a request to join the cluster.
type JoinCommand struct {
	MinVersion int    `json:"minVersion"`
	MaxVersion int    `json:"maxVersion"`
	Name       string `json:"name"`
	RaftURL    string `json:"raftURL"`
	EtcdURL    string `json:"etcdURL"`
}

// ClusterConfig represents cluster-wide configuration settings.
type ClusterConfig struct {
	ActiveSize   int     `json:"activeSize"`
	RemoveDelay  float64 `json:"removeDelay"`
	SyncInterval float64 `json:"syncInterval"`
}

// SetClusterConfigCommand sets the cluster-level configuration.
type SetClusterConfigCommand struct {
	Config *ClusterConfig `json:"config"`
}

// The CompareAndDelete performs a conditional delete on a key in the store.
type CompareAndDeleteCommand struct {
	Key       string `json:"key"`
	PrevValue string `json:"prevValue"`
	PrevIndex uint64 `json:"prevIndex"`
}

// The CompareAndSwap performs a conditional update on a key in the store.
type CompareAndSwapCommand struct {
	Key        string    `json:"key"`
	Value      string    `json:"value"`
	ExpireTime time.Time `json:"expireTime"`
	PrevValue  string    `json:"prevValue"`
	PrevIndex  uint64    `json:"prevIndex"`
}

// Create command
type CreateCommand struct {
	Key        string    `json:"key"`
	Value      string    `json:"value"`
	ExpireTime time.Time `json:"expireTime"`
	Unique     bool      `json:"unique"`
	Dir        bool      `json:"dir"`
}

// The DeleteCommand removes a key from the Store.
type DeleteCommand struct {
	Key       string `json:"key"`
	Recursive bool   `json:"recursive"`
	Dir       bool   `json:"dir"`
}

// Create command
type SetCommand struct {
	Key        string    `json:"key"`
	Value      string    `json:"value"`
	ExpireTime time.Time `json:"expireTime"`
	Dir        bool      `json:"dir"`
}

type SyncCommand struct {
	Time time.Time `json:"time"`
}

// Update command
type UpdateCommand struct {
	Key        string    `json:"key"`
	Value      string    `json:"value"`
	ExpireTime time.Time `json:"expireTime"`
}

// Join command
type DefaultJoinCommand struct {
	Name             string `json:"name"`
	ConnectionString string `json:"connectionString"`
}

// Leave command
type DefaultLeaveCommand struct {
	Name string `json:"name"`
}

// The name of the Leave command in the log
func (c *DefaultLeaveCommand) CommandName() string {
	return "raft:leave"
}

// NOP command
type NOPCommand struct {
}

// The name of the NOP command in the log
func (c NOPCommand) CommandName() string {
	return "raft:nop"
}
