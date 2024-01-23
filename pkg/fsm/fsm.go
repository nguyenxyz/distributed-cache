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
	ErrNotRaftLeader        = errors.New("state machine is not Raft leader")
	ErrUnsupportedOperation = errors.New("event has unsupported operation")
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

	event := Event{
		Operation: "set",
		Key:       key,
		Value:     value,
	}

	return fsm.replicate(event)
}

func (fsm *FiniteStateMachine) Delete(key string) error {
	if !fsm.isRaftLeader() {
		fsm.logger.Errorf("Calling Delete on follower")
		return ErrNotRaftLeader
	}

	event := Event{
		Operation: "delete",
		Key:       key,
	}

	return fsm.replicate(event)
}

// Apply applies an event from the log to the finite state machine and is called once a log entry is committed by a quorum of the cluster
func (fsm *FiniteStateMachine) Apply(l *raft.Log) interface{} {
	var event Event
	if err := json.Unmarshal(l.Data, &event); err != nil {
		fsm.logger.Fatalf("Failed to unmarshal an event from the event log: %v", err)
	}

	switch event.Operation {
	case "set":
		return fsm.Store.Set(event.Key, event.Value)

	case "delete":
		return fsm.Store.Delete(event.Key)

	default:
		return ErrUnsupportedOperation
	}
}

// replicate replicates an event to followers and applies the event to the fsm if it is commited by a quorum of the cluster
func (fsm *FiniteStateMachine) replicate(event Event) error {
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}

	future := fsm.raft.Apply(b, RaftTimeOut)
	if err := future.Error(); err != nil {
		fsm.logger.Errorf("Encountered an error during Raft operation: %v", err)
		return err
	}

	response := future.Response()
	if err, ok := response.(error); ok {
		fsm.logger.Errorf("Encountered an error during FSM operation", err)
		return err
	}

	return nil
}

func (fsm *FiniteStateMachine) isRaftLeader() bool {
	return fsm.raft.State() == raft.Leader
}
