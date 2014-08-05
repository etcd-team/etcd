package raft

const (
	InfoType int64 = iota + 1
	EntryType
	StateType
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
