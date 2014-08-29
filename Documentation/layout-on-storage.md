## Layout on Stable Storage

The document describes the layout of files in data directory on stable storage.

### Log Files Under /wal

All file names are in format "%016x-%016x.wal".

The first number indicates the sequence number of the file, which increases as 0, 1, 2, 3, ...

The second one is the index of the last raft entry written so far.

### Snapshot Files Under /snap

The format for filename is "%016x-%016x-%016x.snap".

The first number is cluster id.

The following two are the term and committed index when the snapshot is generated.

### Append-Only Writes and No File Deletion

Etcd only appends data to files instead of overwriting.

It never deletes any files in the directory.

This ensures that no data is erased forever, and provides atomicity and durability for logs and snapshots.

### Files of Removed Nodes

[Unimplemented]
