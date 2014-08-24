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

package etcdserver

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/coreos/etcd/conf"
	"github.com/coreos/etcd/store"
)

const (
	stateFollower  = "follower"
	stateCandidate = "candidate"
	stateLeader    = "leader"
)

// machineMessage represents information about a peer or standby in the registry.
type machineMessage struct {
	Name      string `json:"name"`
	Id        int64  `json:"id"`
	State     string `json:"state"`
	ClientURL string `json:"clientURL"`
	PeerURL   string `json:"peerURL"`
}

type context struct {
	MinVersion int    `json:"minVersion"`
	MaxVersion int    `json:"maxVersion"`
	Name       string `json:"name"`
	ClientURL  string `json:"clientURL"`
	PeerURL    string `json:"peerURL"`
}

func (p *participant) serveAdminConfig(w http.ResponseWriter, r *http.Request) error {
	switch r.Method {
	case "GET":
	case "PUT":
		if !p.node.IsLeader() {
			return p.redirect(w, r, p.node.Leader())
		}
		c := p.clusterConfig()
		if err := json.NewDecoder(r.Body).Decode(c); err != nil {
			return err
		}
		c.Sanitize()
		if err := p.setClusterConfig(c); err != nil {
			return err
		}
	default:
		return allow(w, "GET", "PUT")
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p.clusterConfig())
	return nil
}

func (p *participant) serveAdminMachines(w http.ResponseWriter, r *http.Request) error {
	idStr := strings.TrimPrefix(r.URL.Path, v2adminMachinesPrefix)
	switch r.Method {
	case "GET":
		var info interface{}
		var err error
		if idStr != "" {
			info, err = p.someMachineMessage(idStr)
		} else {
			info, err = p.allMachineMessages()
		}
		if err != nil {
			return err
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(info)
	case "PUT":
		if !p.node.IsLeader() {
			return p.redirect(w, r, p.node.Leader())
		}
		id, err := strconv.ParseInt(idStr, 0, 64)
		if err != nil {
			return err
		}
		info := &context{}
		if err := json.NewDecoder(r.Body).Decode(info); err != nil {
			return err
		}
		return p.add(&machineMessage{Name: info.Name, Id: id, PeerURL: info.PeerURL, ClientURL: info.ClientURL})
	case "DELETE":
		if !p.node.IsLeader() {
			return p.redirect(w, r, p.node.Leader())
		}
		id, err := strconv.ParseInt(idStr, 0, 64)
		if err != nil {
			return err
		}
		return p.remove(id)
	default:
		return allow(w, "GET", "PUT", "DELETE")
	}
	return nil
}

func (p *participant) clusterConfig() *conf.ClusterConfig {
	c := conf.NewClusterConfig()
	// This is used for backward compatibility because it doesn't
	// set cluster config in older version.
	if e, err := p.Store.Get(v2configKVPrefix, false, false); err == nil {
		json.Unmarshal([]byte(*e.Node.Value), c)
	}
	return c
}

func (p *participant) setClusterConfig(c *conf.ClusterConfig) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if _, err := p.Set(v2configKVPrefix, false, string(b), store.Permanent); err != nil {
		return err
	}
	return nil
}

// someMachineMessage return machine message of specified name.
func (p *participant) someMachineMessage(idStr string) (*machineMessage, error) {
	pp := filepath.Join(v2machineKVPrefix, idStr)
	e, err := p.Store.Get(pp, false, false)
	if err != nil {
		return nil, err
	}
	return newMachineMessage(e.Node, p.node.Leader()), nil
}

func (p *participant) allMachineMessages() ([]*machineMessage, error) {
	e, err := p.Store.Get(v2machineKVPrefix, false, false)
	if err != nil {
		return nil, err
	}
	lead := p.node.Leader()
	ms := make([]*machineMessage, len(e.Node.Nodes))
	for i, n := range e.Node.Nodes {
		ms[i] = newMachineMessage(n, lead)
	}
	return ms, nil
}

func newMachineMessage(n *store.NodeExtern, lead int64) *machineMessage {
	m := new(machineMessage)
	if err := json.Unmarshal([]byte(*n.Value), m); err != nil {
		panic(err)
	}
	if lead == m.Id {
		m.State = stateLeader
	} else {
		m.State = stateFollower
	}
	return m
}
