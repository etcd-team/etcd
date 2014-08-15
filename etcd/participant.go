/*
Copyright 2014 CoreOS Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package etcd

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/coreos/etcd/conf"
	etcdErr "github.com/coreos/etcd/error"
	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/store"
	"github.com/coreos/etcd/wal"
)

const (
	defaultHeartbeat = 1
	defaultElection  = 5
	defaultCompact   = 10000

	maxBufferedProposal = 128

	defaultTickDuration = time.Millisecond * 100

	v2machineKVPrefix = "/_etcd/machines"
	v2configKVPrefix  = "/_etcd/config"

	v2Prefix              = "/v2/keys"
	v2machinePrefix       = "/v2/machines"
	v2peersPrefix         = "/v2/peers"
	v2LeaderPrefix        = "/v2/leader"
	v2SelfStatsPrefix     = "/v2/stats/self"
	v2LeaderStatsPrefix   = "/v2/stats/leader"
	v2StoreStatsPrefix    = "/v2/stats/store"
	v2adminConfigPrefix   = "/v2/admin/config"
	v2adminMachinesPrefix = "/v2/admin/machines/"

	sizeCheckInterval = time.Second
)

var (
	tmpErr      = fmt.Errorf("try again")
	stopErr     = fmt.Errorf("server is stopped")
	raftStopErr = fmt.Errorf("raft is stopped")
	fullErr     = fmt.Errorf("cluster is full")
)

type participant struct {
	id           int64
	clusterId    int64
	cfg          *conf.Config
	pubAddr      string
	raftPubAddr  string
	tickDuration time.Duration

	client  *v2client
	peerHub *peerHub

	proposal    chan v2Proposal
	addNodeC    chan raft.Config
	removeNodeC chan raft.Config
	node        *v2Raft
	store.Store
	rh          *raftHandler
	w           *wal.WAL
	serverStats *raftServerStats

	stopNotifyc chan struct{}

	*http.ServeMux
}

func newParticipant(id int64, c *conf.Config, client *v2client, peerHub *peerHub, tickDuration time.Duration) (*participant, error) {
	p := &participant{
		clusterId:    -1,
		cfg:          c,
		tickDuration: tickDuration,

		client:  client,
		peerHub: peerHub,

		proposal:    make(chan v2Proposal, maxBufferedProposal),
		addNodeC:    make(chan raft.Config, 1),
		removeNodeC: make(chan raft.Config, 1),
		node: &v2Raft{
			result: make(map[wait]chan interface{}),
		},
		Store:       store.New(),
		serverStats: NewRaftServerStats(fmt.Sprint(id)),

		stopNotifyc: make(chan struct{}),

		ServeMux: http.NewServeMux(),
	}
	p.rh = newRaftHandler(peerHub, p.Store.Version(), p.serverStats)
	p.peerHub.setServerStats(p.serverStats)

	walPath := path.Join(p.cfg.DataDir, "wal")
	w, err := wal.Open(walPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}

		p.id = id
		p.pubAddr = c.Addr
		p.raftPubAddr = c.Peer.Addr
		if w, err = wal.New(walPath); err != nil {
			return nil, err
		}
		p.node.Node = raft.New(p.id, defaultHeartbeat, defaultElection)
		info := p.node.Info()
		if err = w.SaveInfo(&info); err != nil {
			return nil, err
		}
		log.Printf("id=%x participant.new path=%s\n", p.id, walPath)
	} else {
		n, err := w.LoadNode()
		if err != nil {
			return nil, err
		}
		p.id = n.Id
		p.node.Node = raft.Recover(n.Id, n.Ents, n.State, defaultHeartbeat, defaultElection)
		p.apply(p.node.Next())
		log.Printf("id=%x participant.load path=%s state=\"%+v\" len(ents)=%d", p.id, walPath, n.State, len(n.Ents))
	}
	p.w = w

	p.Handle(v2Prefix+"/", handlerErr(p.serveValue))
	p.Handle(v2machinePrefix, handlerErr(p.serveMachines))
	p.Handle(v2peersPrefix, handlerErr(p.serveMachines))
	p.Handle(v2LeaderPrefix, handlerErr(p.serveLeader))
	p.Handle(v2SelfStatsPrefix, handlerErr(p.serveSelfStats))
	p.Handle(v2LeaderStatsPrefix, handlerErr(p.serveLeaderStats))
	p.Handle(v2StoreStatsPrefix, handlerErr(p.serveStoreStats))
	p.rh.Handle(v2adminConfigPrefix, handlerErr(p.serveAdminConfig))
	p.rh.Handle(v2adminMachinesPrefix, handlerErr(p.serveAdminMachines))

	// TODO: remind to set application/json for /v2/stats endpoint

	return p, nil
}

func (p *participant) run(stop chan struct{}) {
	defer p.cleanup()

	if p.node.IsEmpty() {
		seeds := p.peerHub.getSeeds()
		if len(seeds) == 0 {
			log.Printf("id=%x participant.run action=bootstrap\n", p.id)
			p.node.Campaign()
			p.node.InitCluster(genId())
			p.node.Add(p.id, p.raftPubAddr, []byte(p.pubAddr))
			p.apply(p.node.Next())
		} else {
			log.Printf("id=%x participant.run action=join seeds=\"%v\"\n", p.id, seeds)
			if err := p.join(); err != nil {
				log.Printf("id=%x participant.run joinErr=%q", p.id, err)
				return
			}
		}
	}

	p.rh.start()
	defer p.rh.stop()

	node := p.node
	recv := p.rh.recv

	ticker := time.NewTicker(p.tickDuration)
	defer ticker.Stop()
	v2SyncTicker := time.NewTicker(time.Millisecond * 500)
	defer v2SyncTicker.Stop()
	v2SizeCheckTicker := time.NewTicker(sizeCheckInterval)
	defer v2SizeCheckTicker.Stop()

	var proposal chan v2Proposal
	var addNodeC, removeNodeC chan raft.Config
	var v2SizeCheckC <-chan time.Time
	for {
		if node.HasLeader() {
			proposal = p.proposal
			addNodeC = p.addNodeC
			removeNodeC = p.removeNodeC
		} else {
			proposal = nil
			addNodeC = nil
			removeNodeC = nil
		}
		if node.IsLeader() {
			v2SizeCheckC = v2SizeCheckTicker.C
		} else {
			v2SizeCheckC = nil
		}
		select {
		case p := <-proposal:
			node.Propose(p)
		case c := <-addNodeC:
			node.UpdateConf(raft.AddNode, &c)
		case c := <-removeNodeC:
			node.UpdateConf(raft.RemoveNode, &c)
		case msg := <-recv:
			node.Step(*msg)
		case <-ticker.C:
			node.Tick()
		case <-v2SyncTicker.C:
			node.Sync()
		case <-v2SizeCheckC:
			p.maybeShrinkSize()
		case <-stop:
			log.Printf("id=%x participant.stop\n", p.id)
			return
		}
		if s := node.UnstableSnapshot(); !s.IsEmpty() {
			if err := p.Recovery(s.Data); err != nil {
				panic(err)
			}
			log.Printf("id=%x recovered index=%d\n", p.id, s.Index)
		}
		p.apply(node.Next())
		ents := node.UnstableEnts()
		p.save(ents, node.UnstableState())
		p.send(node.Msgs())
		if node.IsRemoved() {
			log.Printf("id=%x participant.end\n", p.id)
			return
		}
		if p.node.EntsLen() > defaultCompact {
			d, err := p.Save()
			if err != nil {
				panic(err)
			}
			p.node.Compact(d)
			log.Printf("id=%x compacted index=\n", p.id)
		}
	}
}

func (p *participant) cleanup() {
	p.w.Close()
	close(p.stopNotifyc)
	p.peerHub.stop()
}

func (p *participant) raftHandler() http.Handler {
	return p.rh
}

func (p *participant) add(id int64, raftPubAddr string, pubAddr string) error {
	log.Printf("id=%x participant.add nodeId=%x raftPubAddr=%s pubAddr=%s\n", p.id, id, raftPubAddr, pubAddr)
	pp := path.Join(v2machineKVPrefix, fmt.Sprint(id))

	// The pre-checking gives caller possible hints on why it cannot join cluster.
	// The result could be wrong in rare cases, but it will be corrected in a
	// short time if it still has connection with the cluster.
	_, err := p.Store.Get(pp, false, false)
	if err == nil {
		return nil
	}
	if v, ok := err.(*etcdErr.Error); !ok || v.ErrorCode != etcdErr.EcodeKeyNotFound {
		log.Printf("id=%x participant.add getErr=\"%v\"\n", p.id, err)
		return err
	}
	size := len(p.node.Nodes())
	if size >= p.clusterConfig().ActiveSize {
		log.Printf("id=%x participant.add addNodeErr=%q", p.id, fullErr)
		return etcdErr.NewError(etcdErr.EcodeNoMorePeer, "", uint64(p.node.Applied()))
	}

	w, err := p.Watch(pp, true, false, 0)
	if err != nil {
		log.Printf("id=%x participant.add watchErr=\"%v\"\n", p.id, err)
		return tmpErr
	}

	select {
	case p.addNodeC <- raft.Config{NodeId: id, Addr: raftPubAddr, ExpectedSize: size + 1, Context: []byte(pubAddr)}:
	default:
		w.Remove()
		log.Printf("id=%x participant.add proposeErr=\"unable to send out addNode proposal\"\n", p.id)
		return tmpErr
	}

	select {
	case v := <-w.EventChan:
		if v.Action != store.Set {
			log.Printf("id=%x participant.add watchErr=\"unexpected action\" action=%s", p.id, v.Action)
			return tmpErr
		}
		return nil
	case <-time.After(6 * defaultHeartbeat * p.tickDuration):
		w.Remove()
		log.Printf("id=%x participant.add watchErr=timeout\n", p.id)
		return tmpErr
	case <-p.stopNotifyc:
		return stopErr
	}
}

func (p *participant) remove(id int64) error {
	log.Printf("id=%x participant.remove nodeId=%x\n", p.id, id)
	pp := path.Join(v2machineKVPrefix, fmt.Sprint(id))

	v, err := p.Store.Get(pp, false, false)
	if err != nil {
		return nil
	}

	select {
	case p.removeNodeC <- raft.Config{NodeId: id}:
	default:
		log.Printf("id=%x participant.remove proposeErr=\"unable to send out removeNode proposal\"\n", p.id)
		return tmpErr
	}

	// TODO(xiangli): do not need to watch if the
	// removal target is self
	w, err := p.Watch(pp, true, false, v.Index()+1)
	if err != nil {
		log.Printf("id=%x participant.remove watchErr=\"%v\"\n", p.id, err)
		return tmpErr
	}

	select {
	case v := <-w.EventChan:
		if v.Action == store.Delete {
			return nil
		}
		log.Printf("id=%x participant.remove watchErr=\"unexpected action\" action=%s\n", p.id, v.Action)
		return tmpErr
	case <-time.After(6 * defaultHeartbeat * p.tickDuration):
		w.Remove()
		log.Printf("id=%x participant.remove watchErr=timeout\n", p.id)
		return tmpErr
	case <-p.stopNotifyc:
		return stopErr
	}
}

func (p *participant) apply(ents []raft.Entry) {
	offset := p.node.Applied() - int64(len(ents)) + 1
	for i, ent := range ents {
		switch ent.Type {
		// expose raft entry type
		case raft.Normal:
			if len(ent.Data) == 0 {
				continue
			}
			p.v2apply(offset+int64(i), ent)
		case raft.ClusterInit:
			p.clusterId = p.node.ClusterId()
			log.Printf("id=%x participant.cluster.setId clusterId=%x\n", p.id, p.clusterId)
		case raft.AddNode:
			cfg := new(raft.Config)
			if err := json.Unmarshal(ent.Data, cfg); err != nil {
				log.Printf("id=%x participant.cluster.addNode unmarshalErr=\"%v\"\n", p.id, err)
				break
			}
			peer, err := p.peerHub.add(cfg.NodeId, cfg.Addr)
			if err != nil {
				log.Printf("id=%x participant.cluster.addNode peerAddErr=\"%v\"\n", p.id, err)
				break
			}
			peer.participate()
			pp := path.Join(v2machineKVPrefix, fmt.Sprint(cfg.NodeId))
			p.Store.Set(pp, false, fmt.Sprintf("raft=%v&etcd=%v", cfg.Addr, string(cfg.Context)), store.Permanent)
			if p.id == cfg.NodeId {
				p.raftPubAddr = cfg.Addr
				p.pubAddr = string(cfg.Context)
			}
			log.Printf("id=%x participant.cluster.addNode nodeId=%x addr=%s context=%s\n", p.id, cfg.NodeId, cfg.Addr, cfg.Context)
		case raft.RemoveNode:
			cfg := new(raft.Config)
			if err := json.Unmarshal(ent.Data, cfg); err != nil {
				log.Printf("id=%x participant.cluster.removeNode unmarshalErr=\"%v\"\n", p.id, err)
				break
			}
			peer, err := p.peerHub.peer(cfg.NodeId)
			if err != nil {
				log.Fatal("id=%x participant.apply getPeerErr=\"%v\"", p.id, err)
			}
			peer.idle()
			pp := path.Join(v2machineKVPrefix, fmt.Sprint(cfg.NodeId))
			p.Store.Delete(pp, false, false)
			log.Printf("id=%x participant.cluster.removeNode nodeId=%x\n", p.id, cfg.NodeId)
		default:
			panic("unimplemented")
		}
	}
}

func (p *participant) save(ents []raft.Entry, state raft.State) {
	for _, ent := range ents {
		if err := p.w.SaveEntry(&ent); err != nil {
			log.Panicf("id=%x participant.save saveEntryErr=%q", p.id, err)
		}
	}
	if !state.IsEmpty() {
		if err := p.w.SaveState(&state); err != nil {
			log.Panicf("id=%x participant.save saveStateErr=%q", p.id, err)
		}
	}
	if err := p.w.Sync(); err != nil {
		log.Panicf("id=%x participant.save syncErr=%q", p.id, err)
	}

}

func (p *participant) maybeShrinkSize() {
	nodes := p.node.Nodes()
	activeSize := p.clusterConfig().ActiveSize
	if activeSize <= 0 || len(nodes) <= activeSize {
		return
	}
	log.Printf("id=%x participant.checkActiveSize size=%d expectedSize=%d", p.id, len(nodes), activeSize)
	rmId := nodes[0]
	// It doesn't remove the leader to avoid extra leader change.
	if len(nodes) > 1 && rmId == p.id {
		rmId = nodes[1]
	}
	p.node.UpdateConf(raft.RemoveNode, &raft.Config{NodeId: rmId, ExpectedSize: len(nodes) - 1})
	log.Printf("id=%x participant.checkActiveSize remove=%d", p.id, rmId)
}

func (p *participant) send(msgs []raft.Message) {
	for i := range msgs {
		if err := p.peerHub.send(msgs[i]); err != nil {
			log.Printf("id=%x participant.send err=\"%v\"\n", p.id, err)
		}
	}
}

func (p *participant) join() error {
	info := &context{
		MinVersion: store.MinVersion(),
		MaxVersion: store.MaxVersion(),
		ClientURL:  p.pubAddr,
		PeerURL:    p.raftPubAddr,
	}

	max := p.cfg.MaxRetryAttempts
	for attempt := 0; ; attempt++ {
		for seed := range p.peerHub.getSeeds() {
			if err := p.client.AddMachine(seed, fmt.Sprint(p.id), info); err != nil {
				log.Printf("id=%x participant.join addMachineErr=%q", p.id, err)
				if err.ErrorCode == etcdErr.EcodeNoMorePeer {
					return fullErr
				}
				continue
			}
			return nil
		}
		if attempt == max {
			return fmt.Errorf("etcd: cannot join cluster after %d attempts", max)
		}
		time.Sleep(time.Millisecond * time.Duration(p.cfg.RetryInterval*1000))
	}
}

func genId() int64 {
	r := rand.New(rand.NewSource(int64(time.Now().Nanosecond())))
	return r.Int63()
}
