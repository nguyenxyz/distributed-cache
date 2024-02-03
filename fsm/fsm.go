package fsm

import (
	"encoding/json"
	"errors"
	"io"
	"sync/atomic"
	"time"

	"github.com/hashicorp/raft"

	"github.com/ph-ngn/nanobox/cache"
	"github.com/ph-ngn/nanobox/telemetry"
)

const (
	RaftTimeOut         = 15 * time.Second
	ConfigChangeTimeOut = 0
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

	cap atomic.Int64

	raft *raft.Raft
}

func (fsm *FiniteStateMachine) Get(key string) (cache.Entry, bool) {
	if !fsm.isRaftLeader() {
		return fsm.Cache.Peek(key)
	}

	return fsm.Cache.Get(key)
}

func (fsm *FiniteStateMachine) Set(key string, value []byte) (bool, error) {
	if !fsm.isRaftLeader() {
		telemetry.Log().Errorf("Calling Set on follower")
		return false, ErrNotRaftLeader
	}

	event := Event{
		Op:    "set",
		Key:   key,
		Value: value,
	}

	res, err := fsm.replicateAndApplyOnQuorum(event)
	if err == nil {
		telemetry.Log().Debugf("Succesfully replicate and apply event: %+v", event)
		return res.(bool), nil
	}

	return false, err
}

func (fsm *FiniteStateMachine) Update(key string, value []byte) (bool, error) {
	if !fsm.isRaftLeader() {
		telemetry.Log().Errorf("Calling Update on follower")
		return false, ErrNotRaftLeader
	}

	event := Event{
		Op:    "update",
		Key:   key,
		Value: value,
	}

	res, err := fsm.replicateAndApplyOnQuorum(event)
	if err == nil {
		telemetry.Log().Debugf("Succesfully replicate and apply event: %+v", event)
		return res.(bool), nil
	}

	return false, err
}

func (fsm *FiniteStateMachine) Delete(key string) (bool, error) {
	if !fsm.isRaftLeader() {
		telemetry.Log().Errorf("Calling Delete on follower")
		return false, ErrNotRaftLeader
	}

	event := Event{
		Op:  "delete",
		Key: key,
	}

	res, err := fsm.replicateAndApplyOnQuorum(event)
	if err == nil {
		telemetry.Log().Debugf("Succesfully replicate and apply event: %+v", event)
		return res.(bool), nil
	}

	return false, err
}

func (fsm *FiniteStateMachine) Purge() error {
	if !fsm.isRaftLeader() {
		telemetry.Log().Errorf("Calling Purge on follower")
		return ErrNotRaftLeader
	}

	event := Event{
		Op: "purge",
	}

	_, err := fsm.replicateAndApplyOnQuorum(event)
	if err == nil {
		telemetry.Log().Debugf("Succesfully replicate and apply event: %+v", event)
	}

	return err
}

func (fsm *FiniteStateMachine) Resize(cap int) error {
	if !fsm.isRaftLeader() {
		telemetry.Log().Errorf("Calling Resize on follower")
		return ErrNotRaftLeader
	}

	event := Event{
		Op:    "resize",
		Value: cap,
	}

	_, err := fsm.replicateAndApplyOnQuorum(event)
	if err == nil {
		telemetry.Log().Debugf("Succesfully replicate and apply event: %+v", event)
	}

	return err
}

func (fsm *FiniteStateMachine) UpdateDefaultTTL(ttl time.Duration) error {
	if !fsm.isRaftLeader() {
		telemetry.Log().Errorf("Calling UpdateDefaultTTL on follower")
		return ErrNotRaftLeader
	}

	event := Event{
		Op:    "updatettl",
		Value: ttl.Nanoseconds(),
	}

	_, err := fsm.replicateAndApplyOnQuorum(event)
	if err == nil {
		telemetry.Log().Debugf("Succesfully replicate and apply event: %+v", event)
	}

	return err
}

// Apply applies an event from the log to the finite state machine and is called once a log entry is committed by a quorum of the cluster
func (fsm *FiniteStateMachine) Apply(l *raft.Log) interface{} {
	var event Event
	if err := json.Unmarshal(l.Data, &event); err != nil {
		// need to exit and recover here so the fsm get a chance to reapply the event
		telemetry.Log().Fatalf("Failed to unmarshal an event from the event log: %v", err)
	}

	switch event.Op {
	case "set":
		return fsm.Cache.Set(event.Key, event.Value.([]byte))

	case "update":
		return fsm.Cache.Update(event.Key, event.Value.([]byte))

	case "delete":
		return fsm.Cache.Delete(event.Key)

	case "purge":
		fsm.Cache.Purge()
		return nil

	case "resize":
		cap := event.Value.(int)
		if cap <= 0 {
			cap = -1
		}
		fsm.cap.Store(int64(cap))
		// disable eviction on replicas, but still store the current cap to restore when become leader
		if fsm.isRaftLeader() {
			fsm.Cache.Resize(cap)
		}

		return nil

	case "updatettl":
		fsm.Cache.UpdateDefaultTTL(time.Duration(event.Value.(int64)))
		return nil

	default:
		return ErrUnsupportedOperation
	}
}

func (fsm *FiniteStateMachine) Join(nodeID, addr string) error {
	telemetry.Log().Infof("Received join request from node %s at %s", nodeID, addr)
	configFuture := fsm.raft.GetConfiguration()
	if err := configFuture.Error(); err != nil {
		telemetry.Log().Errorf("Failed to get raft configuration: %v", err)
		return err
	}

	for _, srv := range configFuture.Configuration().Servers {
		if srv.ID == raft.ServerID(nodeID) || srv.Address == raft.ServerAddress(addr) {
			if srv.ID == raft.ServerID(nodeID) && srv.Address == raft.ServerAddress(addr) {
				telemetry.Log().Infof("node %s at %s is already a member", nodeID, addr)
				return nil
			}

			future := fsm.raft.RemoveServer(srv.ID, 0, ConfigChangeTimeOut)
			if err := future.Error(); err != nil {
				telemetry.Log().Errorf("Failed to remove existing node %s at %s: %v", nodeID, addr, err)
				return err
			}
		}
	}

	future := fsm.raft.AddVoter(raft.ServerID(nodeID), raft.ServerAddress(addr), 0, ConfigChangeTimeOut)
	if err := future.Error(); err != nil {
		telemetry.Log().Errorf("Failed to join node %s at %s: %v", nodeID, addr, err)
		return err
	}

	telemetry.Log().Infof("Successfully join node %s at %s", nodeID, addr)
	return nil
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
		telemetry.Log().Errorf("Encountered an error during Raft operation: %v", err)
		return nil, err
	}

	return future.Response(), nil
}

func (fsm *FiniteStateMachine) isRaftLeader() bool {
	return fsm.raft.State() == raft.Leader
}
