package fsm

import (
	"encoding/json"

	"github.com/hashicorp/raft"
	"github.com/ph-ngn/nanobox/cache"
)

type Snapshot struct {
	memory []cache.Entry
}

func (s *Snapshot) Persist(sink raft.SnapshotSink) error {
	err := func() error {
		b, err := json.Marshal(s.memory)
		if err != nil {
			return err
		}

		if _, err := sink.Write(b); err != nil {
			return err
		}

		return sink.Close()

	}()

	if err != nil {
		sink.Cancel()
	}

	return err
}

func (s *Snapshot) Release() {}
