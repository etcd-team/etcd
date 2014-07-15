package etcd

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/coreos/etcd/config"
	etcdErr "github.com/coreos/etcd/error"
	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/store"
)

const (
	defaultHeartbeat = 1
	defaultElection  = 5

	maxBufferedProposal = 100

	defaultTickDuration = time.Millisecond * 100

	v2machineKVPrefix  = "/_etcd/machines"
	v2Prefix           = "/v2/keys"
	v2machinePrefix    = "/v2/machines"
	v2peersPrefix      = "/v2/peers"
	v2LeaderPrefix     = "/v2/leader"
	v2StoreStatsPrefix = "/v2/stats/store"

	v2configKVPrefix      = "/_etcd/config"
	v2adminConfigPrefix   = "/v2/admin/config"
	v2adminMachinesPrefix = "/v2/admin/machines/"

	raftPrefix = "/raft"
)

const (
	participant = iota
	standby
	stop
)

type Server struct {
	// config.Cluster is used iff in standby mode
	config *config.Config

	mode int

	id           int64
	pubAddr      string
	raftPubAddr  string
	tickDuration time.Duration

	// leader is valid iff in standby mode
	leader string
	nodes  map[string]bool

	proposal    chan v2Proposal
	node        *v2Raft
	addNodeC    chan raft.Config
	removeNodeC chan raft.Config
	t           *transporter
	client      *v2client

	store.Store

	modeC chan int
	stop  chan struct{}

	participantHandler http.Handler
	standbyHandler     http.Handler
}

func New(c *config.Config, id int64) *Server {
	if err := c.Sanitize(); err != nil {
		log.Fatalf("failed sanitizing configuration: %v", err)
	}

	tc := &tls.Config{
		InsecureSkipVerify: true,
	}
	var err error
	if c.PeerTLSInfo().Scheme() == "https" {
		tc, err = c.PeerTLSInfo().ClientConfig()
		if err != nil {
			log.Fatal("failed to create raft transporter tls:", err)
		}
	}

	s := &Server{
		config:       c,
		id:           id,
		pubAddr:      c.Addr,
		raftPubAddr:  c.Peer.Addr,
		nodes:        make(map[string]bool),
		tickDuration: defaultTickDuration,
		proposal:     make(chan v2Proposal, maxBufferedProposal),
		node: &v2Raft{
			Node:   raft.New(id, defaultHeartbeat, defaultElection),
			result: make(map[wait]chan interface{}),
		},
		addNodeC:    make(chan raft.Config),
		removeNodeC: make(chan raft.Config),
		t:           newTransporter(tc),
		client:      newClient(tc),

		Store: store.New(),

		modeC: make(chan int, 10),
		stop:  make(chan struct{}),
	}

	for _, seed := range c.Peers {
		s.nodes[seed] = true
	}

	m := http.NewServeMux()
	m.Handle(v2Prefix+"/", handlerErr(s.serveValue))
	m.Handle(v2machinePrefix, handlerErr(s.serveMachines))
	m.Handle(v2peersPrefix, handlerErr(s.serveMachines))
	m.Handle(v2LeaderPrefix, handlerErr(s.serveLeader))
	m.Handle(v2StoreStatsPrefix, handlerErr(s.serveStoreStats))
	m.Handle(v2adminConfigPrefix, handlerErr(s.serveAdminConfig))
	m.Handle(v2adminMachinesPrefix, handlerErr(s.serveAdminMachines))
	s.participantHandler = m
	m = http.NewServeMux()
	m.Handle("/", handlerErr(s.serveRedirect))
	s.standbyHandler = m
	return s
}

func (s *Server) SetTick(d time.Duration) {
	s.tickDuration = d
}

func (s *Server) RaftHandler() http.Handler {
	return http.HandlerFunc(s.ServeHTTPRaft)
}

func (s *Server) ClusterConfig() *config.ClusterConfig {
	c := config.NewClusterConfig()
	// This is used for backward compatibility because it doesn't
	// set cluster config in older version.
	if e, err := s.Get(v2configKVPrefix, false, false); err == nil {
		json.Unmarshal([]byte(*e.Node.Value), c)
	}
	return c
}

func (s *Server) Run() {
	if len(s.config.Peers) == 0 {
		s.Bootstrap()
	} else {
		s.Join()
	}
}

func (s *Server) Stop() {
	close(s.stop)
	s.t.stop()
}

func (s *Server) Bootstrap() {
	log.Println("starting a bootstrap node")
	s.node.Campaign()
	s.node.Add(s.id, s.raftPubAddr, []byte(s.pubAddr))
	s.apply(s.node.Next())
	s.run()
}

func (s *Server) Join() {
	log.Println("joining cluster via peers", s.config.Peers)
	info := &context{
		MinVersion: store.MinVersion(),
		MaxVersion: store.MaxVersion(),
		ClientURL:  s.pubAddr,
		PeerURL:    s.raftPubAddr,
	}

	url := ""
	for i := 0; i < 5; i++ {
		for seed := range s.nodes {
			if err := s.client.AddMachine(seed, fmt.Sprint(s.id), info); err == nil {
				url = seed
				break
			} else {
				log.Println(err)
			}
		}
		if url != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	s.nodes = map[string]bool{url: true}

	s.run()
}

func (s *Server) Add(id int64, raftPubAddr string, pubAddr string) error {
	p := path.Join(v2machineKVPrefix, fmt.Sprint(id))
	index := s.Index()

	_, err := s.Get(p, false, false)
	if err == nil {
		return fmt.Errorf("existed node")
	}
	if v, ok := err.(*etcdErr.Error); !ok || v.ErrorCode != etcdErr.EcodeKeyNotFound {
		return err
	}
	for {
		if s.mode != participant {
			return fmt.Errorf("server is not in participant")
		}
		s.addNodeC <- raft.Config{NodeId: id, Addr: raftPubAddr, Context: []byte(pubAddr)}
		w, err := s.Watch(p, true, false, index+1)
		if err != nil {
			return err
		}
		select {
		case v := <-w.EventChan:
			if v.Action == store.Set {
				return nil
			}
			index = v.Index()
		case <-time.After(4 * defaultHeartbeat * s.tickDuration):
		}
	}
}

func (s *Server) Remove(id int64) error {
	p := path.Join(v2machineKVPrefix, fmt.Sprint(id))
	index := s.Index()

	if _, err := s.Get(p, false, false); err != nil {
		return err
	}
	for {
		if s.mode != participant {
			return fmt.Errorf("server is not in participant")
		}
		s.removeNodeC <- raft.Config{NodeId: id}
		w, err := s.Watch(p, true, false, index+1)
		if err != nil {
			return err
		}
		select {
		case v := <-w.EventChan:
			if v.Action == store.Delete {
				return nil
			}
			index = v.Index()
		case <-time.After(4 * defaultHeartbeat * s.tickDuration):
		}
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch s.mode {
	case participant:
		s.participantHandler.ServeHTTP(w, r)
	case standby:
		s.standbyHandler.ServeHTTP(w, r)
	case stop:
		http.Error(w, "server is stopped", http.StatusInternalServerError)
	}
}

func (s *Server) ServeHTTPRaft(w http.ResponseWriter, r *http.Request) {
	switch s.mode {
	case participant:
		s.t.ServeHTTP(w, r)
	case standby:
		http.NotFound(w, r)
	case stop:
		http.Error(w, "server is stopped", http.StatusInternalServerError)
	}
}
func (s *Server) run() {
	for {
		select {
		case s.modeC <- s.mode:
		default:
		}

		switch s.mode {
		case participant:
			s.runParticipant()
		case standby:
			s.runStandby()
		case stop:
			return
		default:
			panic("unsupport mode")
		}
	}
}

func (s *Server) runParticipant() {
	node := s.node
	addNodeC := s.addNodeC
	removeNodeC := s.removeNodeC
	recv := s.t.recv
	ticker := time.NewTicker(s.tickDuration)
	v2SyncTicker := time.NewTicker(time.Millisecond * 500)

	defer node.CancelAll()

	var proposal chan v2Proposal
	for {
		if node.HasLeader() {
			proposal = s.proposal
		} else {
			proposal = nil
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
		case <-s.stop:
			log.Printf("Node: %d stopped\n", s.id)
			s.mode = stop
			return
		}
		s.apply(node.Next())
		s.send(node.Msgs())
		if node.IsRemoved() {
			break
		}
	}

	log.Printf("Node: %d removed to standby mode\n", s.id)
	if s.node.HasLeader() && !s.node.IsLeader() {
		p := path.Join(v2machineKVPrefix, fmt.Sprint(s.node.Leader()))
		if ev, err := s.Get(p, false, false); err == nil {
			if m, err := url.ParseQuery(*ev.Node.Value); err == nil {
				s.leader = m["raft"][0]
			}
		}
	}
	config := s.ClusterConfig()
	s.config.Cluster.ActiveSize = config.ActiveSize
	s.config.Cluster.RemoveDelay = config.RemoveDelay
	s.config.Cluster.SyncInterval = config.SyncInterval
	s.mode = standby
	return
}

func (s *Server) runStandby() {
	syncDuration := time.Duration(int64(s.config.Cluster.SyncInterval * float64(time.Second)))
	joinCluster := func() error {
		if s.config.Cluster.ActiveSize <= len(s.nodes) {
			return fmt.Errorf("full cluster")
		}
		info := &context{
			MinVersion: store.MinVersion(),
			MaxVersion: store.MaxVersion(),
			ClientURL:  s.pubAddr,
			PeerURL:    s.raftPubAddr,
		}
		if err := s.client.AddMachine(s.leader, fmt.Sprint(s.id), info); err != nil {
			return err
		}
		return nil
	}

	for {
		select {
		case <-time.After(syncDuration):
		case <-s.stop:
			log.Printf("Node: %d stopped\n", s.id)
			s.mode = stop
			return
		}

		if err := s.syncCluster(); err != nil {
			continue
		}
		if err := joinCluster(); err != nil {
			continue
		}
		break
	}

	log.Printf("Node: %d removed to participant mode\n", s.id)
	// TODO(yichengq): use old v2Raft
	// 1. reject proposal in leader state when sm is removed
	// 2. record removeIndex in node to ignore msgDenial and old removal
	s.Store = store.New()
	s.node = &v2Raft{
		Node:   raft.New(s.id, defaultHeartbeat, defaultElection),
		result: make(map[wait]chan interface{}),
	}
	s.mode = participant
	return
}

func (s *Server) apply(ents []raft.Entry) {
	offset := s.node.Applied() - int64(len(ents)) + 1
	if len(ents) > 0 {
		println("node", s.id, "apply", len(ents), "to", s.node.Applied())
	}
	for i, ent := range ents {
		switch ent.Type {
		// expose raft entry type
		case raft.Normal:
			if len(ent.Data) == 0 {
				continue
			}
			s.v2apply(offset+int64(i), ent)
		case raft.AddNode:
			cfg := new(raft.Config)
			if err := json.Unmarshal(ent.Data, cfg); err != nil {
				log.Println(err)
				break
			}
			if err := s.t.set(cfg.NodeId, cfg.Addr); err != nil {
				log.Println(err)
				break
			}
			log.Printf("Add Node %x %v %v\n", cfg.NodeId, cfg.Addr, string(cfg.Context))
			p := path.Join(v2machineKVPrefix, fmt.Sprint(cfg.NodeId))
			if _, err := s.Store.Set(p, false, fmt.Sprintf("raft=%v&etcd=%v", cfg.Addr, string(cfg.Context)), store.Permanent); err == nil {
				s.nodes[cfg.Addr] = true
			}
		case raft.RemoveNode:
			cfg := new(raft.Config)
			if err := json.Unmarshal(ent.Data, cfg); err != nil {
				log.Println(err)
				break
			}
			log.Printf("Remove Node %x\n", cfg.NodeId)
			p := path.Join(v2machineKVPrefix, fmt.Sprint(cfg.NodeId))
			if ev, err := s.Get(p, false, false); err == nil {
				if m, err := url.ParseQuery(*ev.Node.Value); err == nil {
					delete(s.nodes, m["raft"][0])
				}
			}
			s.Store.Delete(p, false, false)
		default:
			panic("unimplemented")
		}
	}
}

func (s *Server) send(msgs []raft.Message) {
	for i := range msgs {
		data, err := json.Marshal(msgs[i])
		if err != nil {
			// todo(xiangli): error handling
			log.Fatal(err)
		}
		// todo(xiangli): reuse routines and limit the number of sending routines
		// sync.Pool?
		go func(i int) {
			var err error
			if err = s.t.sendTo(msgs[i].To, data); err == nil {
				return
			}
			if err == errUnknownNode {
				err = s.fetchAddr(msgs[i].To)
			}
			if err == nil {
				err = s.t.sendTo(msgs[i].To, data)
			}
			if err != nil {
				log.Println(err)
			}
		}(i)
	}
}

func (s *Server) setClusterConfig(c *config.ClusterConfig) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if _, err := s.Set(v2configKVPrefix, false, string(b), store.Permanent); err != nil {
		return err
	}
	return nil
}

func (s *Server) fetchAddr(nodeId int64) error {
	for seed := range s.nodes {
		if err := s.t.fetchAddr(seed, nodeId); err == nil {
			return nil
		}
	}
	return fmt.Errorf("cannot fetch the address of node %d", nodeId)
}
