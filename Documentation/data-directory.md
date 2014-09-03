## Data Directory

Data directory is the place on stable storage to save persistent data.

It plays a *CRITICAL* role for etcd, and etcd makes assumption that a healthy machine always keeps complete and uncorrupted data.

One data directory represents one and only one active etcd machine.

## Layout of Data Directory

The document describes the layout of files in data directory on stable storage.

### Log Files Under /wal

All file names are in format "%016x-%016x.wal".

The first number indicates the sequence number of the file, which increases as 0, 1, 2, 3, ...

The second number is the index of the last raft log entry written.

### Snapshot Files Under /snap

The format for filename is "%016x-%016x-%016x.snap".

The first number is cluster id.

The following two are the term and committed index when the snapshot is generated.

### Append-Only Writes and No File Deletion

etcd only appends data to files instead of overwriting and never deletes any file in the directory. This ensures that no data is erased forever, and provides atomicity and durability for logs and snapshots.
