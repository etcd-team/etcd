package raft

var emptySnapshot = Snapshot{}

func (s Snapshot) IsEmpty() bool {
	return s.Term == 0
}
