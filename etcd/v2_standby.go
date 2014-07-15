package etcd
/*
import (
	"fmt"
	"net/http"
)

func (s *Server) serveRedirect(w http.ResponseWriter, r *http.Request) error {
	if s.leader == "" {
		return fmt.Errorf("no leader in the cluster")
	}
	redirectAddr, err := s.buildRedirectURL(s.leader, r.URL)
	if err != nil {
		return err
	}
	http.Redirect(w, r, redirectAddr, http.StatusTemporaryRedirect)
	return nil
}

func (s *Server) syncCluster() error {
	for node := range s.nodes {
		machines, err := s.client.GetMachines(node)
		if err != nil {
			continue
		}
		config, err := s.client.GetClusterConfig(node)
		if err != nil {
			continue
		}
		s.nodes = make(map[string]bool)
		for _, machine := range machines {
			s.nodes[machine.PeerURL] = true
			if machine.State == stateLeader {
				s.leader = machine.PeerURL
			}
		}
		s.config.Cluster.ActiveSize = config.ActiveSize
		s.config.Cluster.RemoveDelay = config.RemoveDelay
		s.config.Cluster.SyncInterval = config.SyncInterval
		return nil
	}
	return fmt.Errorf("unreachable cluster")
}*/
