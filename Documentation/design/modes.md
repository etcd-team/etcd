## Modes

Etcd can run in either participant mode or standby mode.

### Participant Mode

In this mode, the node enrolls in raft consensus protocol. It maintains a copy of the log in its disk.

### Standby Mode

Standby-mode node proxies all requests to participant-mode nodes, and joins cluster if needed.

### Participant Mode -> Standby Mode

When the node is removed from raft consensus protocol, it comes into standby mode.

Node id, log data and snapshot is discarded because they belongs to a removed node and should not be reused.

### Standby Mode -> Participant Mode

When the node finds there is some vacancy in the cluster, it tries to join the cluster as a new participant.

### Participant Mode -> Stopped

When there is unexpected error from persistent storage or key-value system, etcd should stop immediately and quit.
