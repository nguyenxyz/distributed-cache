package raft

import (
	"encoding/json"

	hraft "github.com/hashicorp/raft"
	"github.com/phonghmnguyen/ke0/cache"
)

type Snapshot struct {
	memory []cache.Entry
}

func (s *Snapshot) Persist(sink hraft.SnapshotSink) error {
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
