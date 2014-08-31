## Modes

Etcd runs in either participant mode or standby mode.

### Participant Mode

In this mode, etcd machine enrolls in raft consensus protocol. It keeps maintaining a copy of the log in its disk.

### Standby Mode

Standby-mode machines proxy all requests to participant-mode machines and join the cluster if there is space for more participants.

### Participant Mode -> Standby Mode

When the machine is removed from raft consensus protocol, it comes into standby mode.

NodeId, log data and snapshot is discarded because they belongs to a removed machine and should not be reused.

### Standby Mode -> Participant Mode

When the machine finds there is some vacancy in the cluster, it tries to join the cluster as a new participant.

### Participant Mode -> Stopped

When there is unexpected error from persistent storage or key-value system, etcd should stop immediately and quit.
