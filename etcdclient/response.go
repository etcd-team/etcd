package etcdclient

import "time"

type Response struct {
	Action   string
	Node     *Node
	PrevNode *Node
}

type Node struct {
	Key           string
	Value         string
	Dir           bool
	Expiration    *time.Time
	TTL           int64
	Nodes         Nodes
	ModifiedIndex uint64
	CreatedIndex  uint64
}

type Nodes []*Node
