package raft

var (
	InfoType  = int64(1)
	EntryType = int64(2)
	StateType = int64(3)
)

type Marshaler interface {
	Marshal() ([]byte, error)
	MarshalType() int64
}

func (i *Info) MarshalType() int64 {
	return InfoType
}

func (e *Entry) MarshalType() int64 {
	return EntryType
}

func (s *State) MarshalType() int64 {
	return StateType
}
