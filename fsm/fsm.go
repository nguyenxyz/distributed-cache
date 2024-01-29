package fsm

import (
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/hashicorp/raft"

	"github.com/ph-ngn/nanobox/cache"
	"github.com/ph-ngn/nanobox/telemetry"
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
	Op    string      `json:"op,omitempty"`
	Key   string      `json:"key,omitempty"`
	Value interface{} `json:"value,omitempty"`
}

// FiniteStateMachine is a wrapper around Store and manages replication with Raft consensus
type FiniteStateMachine struct {
	cache.Cache

	raft *raft.Raft

	logger telemetry.Logger
}

func (fsm *FiniteStateMachine) Set(key string, value interface{}) (bool, error) {
	if !fsm.isRaftLeader() {
		fsm.logger.Errorf("Calling Set on follower")
		return false, ErrNotRaftLeader
	}

	event := Event{
		Op:    "set",
		Key:   key,
		Value: value,
	}

	res, err := fsm.replicateAndApplyOnQuorum(event)
	if err != nil {
		fsm.logger.Debugf("Succesfully replicate and apply event: %+v", event)
		return res.(bool), nil
	}

	return false, err
}

func (fsm *FiniteStateMachine) Delete(key string) (bool, error) {
	if !fsm.isRaftLeader() {
		fsm.logger.Errorf("Calling Delete on follower")
		return false, ErrNotRaftLeader
	}

	event := Event{
		Op:  "delete",
		Key: key,
	}

	res, err := fsm.replicateAndApplyOnQuorum(event)
	if err != nil {
		fsm.logger.Debugf("Succesfully replicate and apply event: %+v", event)
		return res.(bool), nil
	}

	return false, err
}

func (fsm *FiniteStateMachine) Purge() error {
	if !fsm.isRaftLeader() {
		fsm.logger.Errorf("Calling Purge on follower")
		return ErrNotRaftLeader
	}

	event := Event{
		Op: "purge",
	}

	_, err := fsm.replicateAndApplyOnQuorum(event)
	if err != nil {
		fsm.logger.Debugf("Succesfully replicate and apply event: %+v", event)
	}

	return err
}

// Apply applies an event from the log to the finite state machine and is called once a log entry is committed by a quorum of the cluster
func (fsm *FiniteStateMachine) Apply(l *raft.Log) interface{} {
	var event Event
	if err := json.Unmarshal(l.Data, &event); err != nil {
		// need to exit and recover here so the fsm get a chance to reapply the event
		fsm.logger.Fatalf("Failed to unmarshal an event from the event log: %v", err)
	}

	switch event.Op {
	case "set":
		return fsm.Cache.Set(event.Key, event.Value)

	case "delete":
		return fsm.Cache.Delete(event.Key)

	case "purge":
		fsm.Cache.Purge()
		return nil

	default:
		return ErrUnsupportedOperation
	}
}

func (fsm *FiniteStateMachine) Snapshot() (raft.FSMSnapshot, error) {
	return nil, nil
}

func (fsm *FiniteStateMachine) Restore(snapshot io.ReadCloser) error {
	return nil
}

// replicateAndApplyOnQuorum replicates an event to followers and applies the event to the fsm if it is commited by a quorum of the cluster
func (fsm *FiniteStateMachine) replicateAndApplyOnQuorum(event Event) (interface{}, error) {
	b, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}

	future := fsm.raft.Apply(b, RaftTimeOut)
	if err := future.Error(); err != nil {
		fsm.logger.Errorf("Encountered an error during Raft operation: %v", err)
		return nil, err
	}

	return future.Response(), nil
}

func (fsm *FiniteStateMachine) isRaftLeader() bool {
	return fsm.raft.State() == raft.Leader
}
