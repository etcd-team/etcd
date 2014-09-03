package etcd

import (
	"encoding/json"
	"fmt"

	"github.com/coreos/etcd/goraft"
	"github.com/coreos/etcd/goraft/protobuf"
	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/store"
	"github.com/coreos/etcd/wal"
)

func LoadGoraftDataDir(dirpath string, name string) (*wal.Node, error) {
	node := &wal.Node{}
	dataset, err := goraft.LoadDataset(dirpath)
	if err != nil {
		return nil, err
	}

	if isNameInPeers(name, dataset.Config.Peers) {
		return nil, fmt.Errorf("wal: name doesn't exist in peer list")
	}
	node.Id = hash(name)

	var index uint64
	for _, e := range dataset.Entries {
		if e.GetIndex() != index+1 {
			panic("not increment one by one")
		}
		index++
		ent, err := toEntry(e)
		if err != nil {
			return nil, err
		}
		node.Ents = append(node.Ents, ent)
	}

	node.State = raft.State{Term: 0, Vote: raft.NoneId, Commit: int64(dataset.Config.CommitIndex)}
	return node, nil
}

func isNameInPeers(name string, peers []*goraft.Peer) bool {
	for _, p := range peers {
		if name == p.Name {
			return true
		}
	}
	return false
}

func hash(name string) int64 {
	var sum int64
	for _, ch := range name {
		sum = 131*sum + int64(ch)
	}
	return sum
}

func toEntry(oent *protobuf.LogEntry) (raft.Entry, error) {
	nent := raft.Entry{Term: int64(oent.GetTerm()), Index: int64(oent.GetIndex())}
	cmd, err := goraft.NewCommand(oent.GetCommandName(), oent.GetCommand())
	if err != nil {
		return nent, err
	}
	switch v := cmd.(type) {
	case *goraft.RemoveCommand:
		nent.Type = raft.RemoveNode
		cfg := &raft.Config{NodeId: hash(v.Name)}
		data, err := json.Marshal(cfg)
		if err != nil {
			panic(err)
		}
		nent.Data = data
	case *goraft.JoinCommand:
		nent.Type = raft.AddNode
		cfg := &raft.Config{NodeId: hash(v.Name), Addr: v.RaftURL, Context: []byte(v.EtcdURL)}
		data, err := json.Marshal(cfg)
		if err != nil {
			panic(err)
		}
		nent.Data = data
	case *goraft.SetClusterConfigCommand:
		nent.Type = raft.Normal
		b, err := json.Marshal(v.Config)
		if err != nil {
			panic(err)
		}
		dir := false
		val := string(b)
		ncmd := &Cmd{
			Type:  stset,
			Key:   "/v2/admin/config",
			Dir:   &dir,
			Value: &val,
			Time:  mustMarshalTime(&store.Permanent),
		}
		data, err := ncmd.Marshal()
		if err != nil {
			panic(err)
		}
		nent.Data = data
	case *goraft.CompareAndDeleteCommand:
		nent.Type = raft.Normal
		ncmd := &Cmd{
			Type:      stcad,
			Key:       v.Key,
			PrevValue: &v.PrevValue,
			PrevIndex: &v.PrevIndex,
		}
		data, err := ncmd.Marshal()
		if err != nil {
			panic(err)
		}
		nent.Data = data
	case *goraft.CompareAndSwapCommand:
		nent.Type = raft.Normal
		ncmd := &Cmd{
			Type:      stcas,
			Key:       v.Key,
			Value:     &v.Value,
			PrevValue: &v.PrevValue,
			PrevIndex: &v.PrevIndex,
			Time:      mustMarshalTime(&v.ExpireTime),
		}
		data, err := ncmd.Marshal()
		if err != nil {
			panic(err)
		}
		nent.Data = data
	case *goraft.CreateCommand:
		nent.Type = raft.Normal
		ncmd := &Cmd{
			Type:   stcreate,
			Key:    v.Key,
			Dir:    &v.Dir,
			Value:  &v.Value,
			Time:   mustMarshalTime(&v.ExpireTime),
			Unique: &v.Unique,
		}
		data, err := ncmd.Marshal()
		if err != nil {
			panic(err)
		}
		nent.Data = data
	case *goraft.DeleteCommand:
		nent.Type = raft.Normal
		ncmd := &Cmd{
			Type:      stdelete,
			Key:       v.Key,
			Dir:       &v.Dir,
			Recursive: &v.Recursive,
		}
		data, err := ncmd.Marshal()
		if err != nil {
			panic(err)
		}
		nent.Data = data
	case *goraft.SetCommand:
		nent.Type = raft.Normal
		ncmd := &Cmd{
			Type:  stset,
			Key:   v.Key,
			Dir:   &v.Dir,
			Value: &v.Value,
			Time:  mustMarshalTime(&v.ExpireTime),
		}
		data, err := ncmd.Marshal()
		if err != nil {
			panic(err)
		}
		nent.Data = data
	case *goraft.SyncCommand:
		nent.Type = raft.Normal
		ncmd := &Cmd{
			Type: stsync,
			Time: mustMarshalTime(&v.Time),
		}
		data, err := ncmd.Marshal()
		if err != nil {
			panic(err)
		}
		nent.Data = data
	case *goraft.UpdateCommand:
		nent.Type = raft.Normal
		ncmd := &Cmd{
			Type:  stupdate,
			Key:   v.Key,
			Value: &v.Value,
			Time:  mustMarshalTime(&v.ExpireTime),
		}
		data, err := ncmd.Marshal()
		if err != nil {
			panic(err)
		}
		nent.Data = data
	case *goraft.DefaultJoinCommand:
		panic("unimplemented")
	case *goraft.DefaultLeaveCommand:
		panic("unimplemented")
	case *goraft.NOPCommand:
		nent.Type = raft.Normal
		nent.Data = nil
	default:
		panic("unhandalbe command")
	}
	return nent, nil
}
