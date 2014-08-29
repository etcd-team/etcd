## Discussion on Raft Log File

### Data to Save

- nodeId

NodeId makes etcd know its identity. This is important for debug and misopeartion avoidance.

- currentTerm, voteFor

These two fields are saved in the implementation of raft paper.

They are expected to happen infrequently, so it won't affect performance much.

- commitIndex when cluster configuration changes

When etcd recovers from raft log, it has to know address of its peers. The current solution is replaying enough log entries to load all cluster info. This requires commitIndex to indicate how many entries should be replayed.

This is not required in raft paper, but is necessary here because cluster info is the part of raft log.

- log entries

### Write-ahead logging

WAL indicates that all modifications are appended to a log before they are used, which provides atomicity and durability.

The disadvantage lies on bigger file size, which may increase I/O burdens. It is not a big case for current etcd.

#### Implementation

- Info which consists of nodeId is written iff a new file is opened.

- State, including currentTerm, voteFor and commitIndex, is written whenever it changes.

- Log entries are written when received. The special case is that conflicted entries should be ignored.

### Restart Using Other's Data (Future Plan)

The feature is helpful when user wants to start etcd with files moved manually instead of automatic snapshot mechanism.

One solution is generating one file which contains commitIndex and log entries, and the second one that records other data. When user wants to do it manually, it only moves the first one into target directory and restarts.

The other one is to write some tool which could covert our raft log file into some format that doesn't have unique info.

The second solution is used now because we want to maintain one data file only.

### Integrity

Integrity ensures completeness and correctness of raft log.

#### Implementation

The current way is adding crc32 to each entry, which is calculated based on previous crc32 and data field.

It is necessary to check each entry because it will break raft if any is corrupted.

### Performance (Future Plan)

It is critical to save data into disk fast enough for each step.

It is great if the whole data could be loaded quickly on recovery.

### Multiple Files Design

File name consists of:
1. sequence number
2. the index of the last raft entry before writing to this file

If etcd wants to get all entries starting from some index, it goes through all file names in the order of sequence number, and finds the array index of the last name that has a smaller raft index section than the given one.

## Discussion on Snapshot File

### Separate Snapshot From Log

Reasons are as follows:
1. Support do mmap() on snapshot only
2. Transfer snapshot to other machines
3. Snapshot could be much bigger than log if there are many keys in the store

Its disadvantages are that etcd needs extra logic to combine two parts, and it makes admins harder to understand layout.

### Data To Save

clusterId, peerList, commitIndex, term, storeData

### Performance (Future Plan)

It is great if the whole data could be loaded quickly on recovery.
