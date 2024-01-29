package fsm

import (
	"github.com/hashicorp/raft"
)

type Snapshot struct {
}

func (s *Snapshot) Persist(sink raft.SnapshotSink) error {
	return nil
}

func (s *Snapshot) Release() {

}
