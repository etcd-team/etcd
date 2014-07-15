package etcd

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"testing"
	"time"

	"github.com/coreos/etcd/config"
)

func TestMultipleNodes(t *testing.T) {
	tests := []int{1, 3, 5, 9, 11}

	for _, tt := range tests {
		es, hs := buildCluster(tt, false)
		waitCluster(t, es)
		for i := range es {
			es[len(es)-i-1].Stop()
		}
		for i := range hs {
			hs[len(hs)-i-1].Close()
		}
	}
	afterTest(t)
}

func TestMultipleTLSNodes(t *testing.T) {
	tests := []int{1, 3, 5}

	for _, tt := range tests {
		es, hs := buildCluster(tt, true)
		waitCluster(t, es)
		for i := range es {
			es[len(es)-i-1].Stop()
		}
		for i := range hs {
			hs[len(hs)-i-1].Close()
		}
	}
	afterTest(t)
}

func TestV2Redirect(t *testing.T) {
	es, hs := buildCluster(3, false)
	waitCluster(t, es)
	u := hs[1].URL
	ru := fmt.Sprintf("%s%s", hs[0].URL, "/v2/keys/foo")
	tc := NewTestClient()

	v := url.Values{}
	v.Set("value", "XXX")
	resp, _ := tc.PutForm(fmt.Sprintf("%s%s", u, "/v2/keys/foo"), v)
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusTemporaryRedirect)
	}
	location, err := resp.Location()
	if err != nil {
		t.Errorf("want err = %, want nil", err)
	}

	if location.String() != ru {
		t.Errorf("location = %v, want %v", location.String(), ru)
	}

	resp.Body.Close()
	for i := range es {
		es[len(es)-i-1].Stop()
	}
	for i := range hs {
		hs[len(hs)-i-1].Close()
	}
	afterTest(t)
}

func TestAdd(t *testing.T) {
	tests := []struct {
		size  int
		round int
	}{
		{3, 5},
		{4, 5},
		{5, 5},
		{6, 5},
	}

	for _, tt := range tests {
		es := make([]*Server, tt.size)
		hs := make([]*httptest.Server, tt.size)
		for i := 0; i < tt.size; i++ {
			c := config.New()
			if i > 0 {
				c.Peers = []string{hs[0].URL}
			}
			es[i], hs[i] = initTestServer(c, int64(i), false)
		}

		go es[0].Bootstrap()

		for i := 1; i < tt.size; i++ {
			var index uint64
			for {
				lead := es[0].node.Leader()
				if lead != -1 {
					index = es[lead].Index()
					ne := es[i]
					if err := es[lead].Add(ne.id, ne.raftPubAddr, ne.pubAddr); err == nil {
						break
					}
				}
				runtime.Gosched()
			}
			go es[i].run()

			for j := 0; j <= i; j++ {
				w, err := es[j].Watch(v2machineKVPrefix, true, false, index+1)
				if err != nil {
					t.Errorf("#%d on %d: %v", i, j, err)
					break
				}
				v := <-w.EventChan
				ww := fmt.Sprintf("%s/%d", v2machineKVPrefix, i)
				if v.Node.Key != ww {
					t.Errorf("#%d on %d: path = %v, want %v", i, j, v.Node.Key, ww)
				}
			}
		}

		for i := range hs {
			es[len(hs)-i-1].Stop()
		}
		for i := range hs {
			hs[len(hs)-i-1].Close()
		}
		afterTest(t)
	}
}

func TestRemove(t *testing.T) {
	tests := []struct {
		size  int
		round int
	}{
		{3, 5},
		{4, 5},
		{5, 5},
		{6, 5},
	}

	for _, tt := range tests {
		es, hs := buildCluster(tt.size, false)
		waitCluster(t, es)

		// we don't remove the machine from 2-node cluster because it is
		// not 100 percent safe in our raft.
		// TODO(yichengq): improve it later.
		for i := 0; i < tt.size-2; i++ {
			id := int64(i)
			var index uint64
			for {
				lead := es[id].node.Leader()
				if lead != -1 {
					index = es[lead].Index()
					if err := es[lead].Remove(id); err == nil {
						break
					}
				}
				runtime.Gosched()
			}

			// i-th machine cannot be promised to apply the removal command of
			// its own due to our non-optimized raft.
			// TODO(yichengq): it should work when
			// https://github.com/etcd-team/etcd/pull/7 is merged.
			for j := i + 1; j < tt.size; j++ {
				w, err := es[j].Watch(v2machineKVPrefix, true, false, index+1)
				if err != nil {
					t.Errorf("#%d on %d: %v", i, j, err)
					break
				}
				v := <-w.EventChan
				ww := fmt.Sprintf("%s/%d", v2machineKVPrefix, i)
				if v.Node.Key != ww {
					t.Errorf("#%d on %d: path = %v, want %v", i, j, v.Node.Key, ww)
				}

				wsize := tt.size - i - 1
				if g := len(es[j].nodes); g != wsize {
					t.Errorf("#%d on %d: len(nodes) = %d, want %d", i, j, g, wsize)
				}
			}

			// may need to wait for msgDenial
			// TODO(yichengq): no need to sleep here when previous issue is merged.
			if es[i].mode == standby {
				continue
			}
			time.Sleep(defaultElection * defaultTickDuration)
			if g := es[i].mode; g != standby {
				t.Errorf("#%d: mode = %d, want standby", i, g)
			}
		}

		for i := range hs {
			es[len(hs)-i-1].Stop()
		}
		for i := range hs {
			hs[len(hs)-i-1].Close()
		}
		afterTest(t)
	}
}

func TestModeSwitch(t *testing.T) {
	size := 5
	round := 3

	tc := NewTestClient()
	for i := 0; i < size; i++ {
		es, hs := buildCluster(size, false)
		waitCluster(t, es)

		if g := <-es[i].modeC; g != participant {
			t.Fatalf("#%d: mode = %d, want participant", i, g)
		}

		lead, _ := waitLeader(t, es)
		config := config.NewClusterConfig()
		config.SyncInterval = 0

		for j := 0; j < round; j++ {
			config.ActiveSize = size - 1
			if err := es[lead].setClusterConfig(config); err != nil {
				t.Fatalf("#%d: setClusterConfig err = %v", i, err)
			}
			resp, _ := tc.Delete(hs[lead].URL+v2adminMachinesPrefix+fmt.Sprint(i), "", nil)
			tc.ReadBody(resp)
			if g := resp.StatusCode; g != 200 {
				t.Fatalf("#%d: status = %d, want 200", i, g)
			}

			if g := <-es[i].modeC; g != standby {
				t.Fatalf("#%d: mode = %d, want standby", i, g)
			}
			if g := len(es[i].modeC); g != 0 {
				t.Fatalf("#%d: mode to %d, want remain", i, <-es[i].modeC)
			}

			if err := checkAlive(t, i, es); err != nil {
				t.Errorf("#%d %v", i, err)
			}

			lead, _ = waitLeader(t, es)
			config.ActiveSize = size
			if err := es[lead].setClusterConfig(config); err != nil {
				t.Fatalf("#%d: setClusterConfig err = %v", i, err)
			}

			if g := <-es[i].modeC; g != participant {
				t.Fatalf("#%d: mode = %d, want participant", i, g)
			}
			if g := len(es[i].modeC); g != 0 {
				t.Fatalf("#%d: mode to %d, want remain", i, <-es[i].modeC)
			}

			if err := checkAlive(t, i, es); err != nil {
				t.Errorf("#%d %v", i, err)
			}
		}

		if g := len(es[i].modeC); g != 0 {
			t.Fatalf("#%d: mode to %d, want remain", i, <-es[i].modeC)
		}

		for i := range hs {
			es[len(hs)-i-1].Stop()
		}
		for i := range hs {
			hs[len(hs)-i-1].Close()
		}
	}
	afterTest(t)
}

func buildCluster(number int, tls bool) ([]*Server, []*httptest.Server) {
	bootstrapper := 0
	es := make([]*Server, number)
	hs := make([]*httptest.Server, number)
	var seed string

	for i := range es {
		c := config.New()
		if seed != "" {
			c.Peers = []string{seed}
		}
		es[i], hs[i] = initTestServer(c, int64(i), tls)

		if i == bootstrapper {
			seed = hs[i].URL
			go es[i].Bootstrap()
		} else {
			// wait for the previous configuration change to be committed
			// or this configuration request might be dropped
			w, err := es[0].Watch(v2machineKVPrefix, true, false, uint64(i))
			if err != nil {
				panic(err)
			}
			<-w.EventChan
			go es[i].Join()
		}
	}
	return es, hs
}

func initTestServer(c *config.Config, id int64, tls bool) (e *Server, h *httptest.Server) {
	e = New(c, id)
	e.SetTick(time.Millisecond * 5)
	m := http.NewServeMux()
	m.Handle("/", e)
	m.Handle("/raft", e.t)
	m.Handle("/raft/", e.t)

	if tls {
		h = httptest.NewTLSServer(m)
	} else {
		h = httptest.NewServer(m)
	}

	e.raftPubAddr = h.URL
	e.pubAddr = h.URL
	return
}

func waitCluster(t *testing.T, es []*Server) {
	n := len(es)
	for i, e := range es {
		var index uint64
		for k := 0; k < n; k++ {
			index++
			w, err := e.Watch(v2machineKVPrefix, true, false, index)
			if err != nil {
				panic(err)
			}
			v := <-w.EventChan
			// join command may appear several times due to retry
			// when timeout
			if k > 0 {
				pw := fmt.Sprintf("%s/%d", v2machineKVPrefix, k-1)
				if v.Node.Key == pw {
					continue
				}
			}
			ww := fmt.Sprintf("%s/%d", v2machineKVPrefix, k)
			if v.Node.Key != ww {
				t.Errorf("#%d path = %v, want %v", i, v.Node.Key, ww)
			}
		}
	}
}

// checkAlive checks the i-th server is still alive in the cluster.
func checkAlive(t *testing.T, i int, es []*Server) error {
	tc := NewTestClient()
	key := fmt.Sprintf("/%d", rand.Int31())

	lead, _ := waitLeader(t, es)
	resp, _ := tc.PutForm(es[lead].pubAddr+v2Prefix+key, url.Values(map[string][]string{"value": {"bar"}}))
	tc.ReadBody(resp)
	if g := resp.StatusCode; g != 201 {
		return fmt.Errorf("on %d: status = %d, want 201", lead, g)
	}
	index := resp.Header.Get("X-Etcd-Index")

	waitApplied(t, es)
	resp, err := tc.Get(es[i].pubAddr + v2Prefix + key)
	if err != nil {
		return fmt.Errorf("on %d: get err = %v", i, err)
	}
	if g := resp.StatusCode; g != 200 {
		return fmt.Errorf("on %d: status = %d, want 200", i, g)
	}
	w := fmt.Sprintf(`{"action":"get","node":{"key":"%s","value":"bar","modifiedIndex":%s,"createdIndex":%s}}`, key, index, index)
	if g := string(tc.ReadBody(resp)); g != w {
		return fmt.Errorf("on %d: resp = %s, want %s", i, g, w)
	}
	return nil
}

// waitApplied waits until all alive servers catch up with leader.
func waitApplied(t *testing.T, es []*Server) (applied int64) {
	lead, _ := waitLeader(t, es)
	applied = es[lead].node.Applied()
	tickDur := es[0].tickDuration
	check := func(i int) error {
		if es[i].mode != participant {
			return nil
		}
		n := es[i].node
		for j := 0; j < 10; j++ {
			if n.Applied() >= applied {
				return nil
			}
			time.Sleep(defaultHeartbeat * tickDur)
		}
		return fmt.Errorf("on %d: applied = %d, want %d", n.Applied(), applied)
	}
	for i := 0; i < len(es); i++ {
		if err := check(i); err != nil {
			t.Fatalf("wait applied #%d: %v", i, err)
		}
	}
	return
}

// waitLeader waits until all alive servers are checked to have the same leader.
// The lead returned is not guaranteed to be in leader state still.
func waitLeader(t *testing.T, es []*Server) (lead, term int64) {
	tickDur := es[0].tickDuration
	errs := make([]error, 0)
	checkLeaders := func() int64 {
		first := true
		addr := ""
		for j := 0; j < len(es); j++ {
			if es[j].mode != participant {
				continue
			}
			nlead := es[j].node.Leader()
			nterm := es[j].node.Term()
			if nlead == -1 {
				errs = append(errs, fmt.Errorf("on %d: no leader", j))
				return defaultHeartbeat
			}
			if first {
				lead = nlead
				if es[lead].mode != participant {
					errs = append(errs, fmt.Errorf("on %d: leader that is just removed", j))
					return defaultElection
				}
				term = nterm
				addr = es[lead].raftPubAddr
				first = false
			} else if nlead != lead || nterm != term {
				errs = append(errs, fmt.Errorf("on %d: leader = %d, want %d", j, nlead, lead))
				return defaultHeartbeat
			}
		}
		for j := 0; j < len(es); j++ {
			if es[j].mode == standby && es[j].leader != addr {
				errs = append(errs, fmt.Errorf("on %d: leader = %s, want %s", j, es[j].leader, addr))
				rate := float64(time.Second / es[0].tickDuration)
				return int64(es[j].config.Cluster.SyncInterval * rate)
			}
		}
		return 0
	}

	for i := 0; i < 10; i++ {
		tick := checkLeaders()
		if tick == 0 {
			return
		}
		time.Sleep(time.Duration(tick) * tickDur)
	}
	for i, err := range errs {
		t.Errorf("wait leader #%d: %v", i, err)
	}
	t.Fatalf("cannot find the leader of the cluster")
	return
}
