package fsm

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/hashicorp/raft"

	"github.com/ph-ngn/nanobox/pkg/box"
	"github.com/ph-ngn/nanobox/pkg/util/log"
)

const (
	RaftTimeOut = 15 * time.Second
)

var (
	ErrNotRaftLeader = errors.New("state machine is not Raft leader")
)

// Event represents an event in the event log that will get replicated to Raft followers
type Event struct {
	Operation string      `json:"operation,omitempty"`
	Key       string      `json:"key,omitempty"`
	Value     interface{} `json:"value,omitempty"`
}

// FiniteStateMachine is a wrapper around Store and manages replication with Raft consensus
type FiniteStateMachine struct {
	box.Store

	raft *raft.Raft

	logger log.Logger
}

func (fsm *FiniteStateMachine) Set(key string, value interface{}) error {
	if !fsm.isRaftLeader() {
		fsm.logger.Errorf("Calling Set on follower")
		return ErrNotRaftLeader
	}

	event := &Event{
		Operation: "set",
		Key:       key,
		Value:     value,
	}

	b, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return fsm.raft.Apply(b, RaftTimeOut).Error()
}

func (fsm *FiniteStateMachine) Delete(key string) error {
	if !fsm.isRaftLeader() {
		fsm.logger.Errorf("Calling Delete on follower")
		return ErrNotRaftLeader
	}

	event := &Event{
		Operation: "delete",
		Key:       key,
	}

	b, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return fsm.raft.Apply(b, RaftTimeOut).Error()
}

func (fsm *FiniteStateMachine) isRaftLeader() bool {
	return fsm.raft.State() == raft.Leader
}
