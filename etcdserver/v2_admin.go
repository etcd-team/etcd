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
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/coreos/etcd/conf"
	etcdErr "github.com/coreos/etcd/error"
	"github.com/coreos/etcd/store"
)

const (
	stateFollower  = "follower"
	stateCandidate = "candidate"
	stateLeader    = "leader"
)

// machineInfo represents information about a peer or standby in the registry.
type machineInfo struct {
	Name      string `json:"name"`
	Id        int64  `json:"id"`
	State     string `json:"state"`
	ClientURL string `json:"clientURL"`
	PeerURL   string `json:"peerURL"`
}

type machineAttribute struct {
	Name      string `json:"name"`
	ClientURL string `json:"clientURL"`
	PeerURL   string `json:"peerURL"`
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
		if idStr != "" {
			id, err := strconv.ParseInt(idStr, 0, 64)
			if err != nil {
				return err
			}
			info = p.readMachineInfo(id, p.node.Leader())
		} else {
			info = p.readAllMachineInfos()
		}
		if info == nil {
			return fmt.Errorf("nonexist")
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
		ma := &machineAttribute{}
		if err := json.NewDecoder(r.Body).Decode(ma); err != nil {
			return err
		}
		return p.add(id, ma)
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

func (p *participant) readAllMachineInfos() []*machineInfo {
	ev, err := p.Store.Get(v2machineKVPrefix, false, false)
	if err != nil {
		if e, ok := err.(*etcdErr.Error); !ok || e.ErrorCode != etcdErr.EcodeKeyNotFound {
			panic(err.Error())
		}
		return nil
	}
	lead := p.node.Leader()
	mis := make([]*machineInfo, len(ev.Node.Nodes))
	for i, n := range ev.Node.Nodes {
		id, err := strconv.ParseInt(path.Base(n.Key), 0, 64)
		if err != nil {
			panic(err.Error())
		}
		mis[i] = p.readMachineInfo(id, lead)
	}
	return mis
}

func (p *participant) readMachineInfo(id int64, lead int64) *machineInfo {
	ma := p.readMachineAttribute(id)
	if ma == nil {
		return nil
	}
	mi := &machineInfo{
		Name:      ma.Name,
		Id:        id,
		ClientURL: ma.ClientURL,
		PeerURL:   ma.PeerURL,
	}
	if lead == id {
		mi.State = stateLeader
	} else {
		mi.State = stateFollower
	}
	return mi
}

func (p *participant) readMachineAttribute(id int64) *machineAttribute {
	ev, err := p.Store.Get(path.Join(v2machineKVPrefix, fmt.Sprint(id)), false, false)
	if err != nil {
		if e, ok := err.(*etcdErr.Error); !ok || e.ErrorCode != etcdErr.EcodeKeyNotFound {
			panic(err.Error())
		}
		return nil
	}
	ma := new(machineAttribute)
	if err := json.Unmarshal([]byte(*ev.Node.Value), ma); err != nil {
		panic(err)
	}
	return ma
}
