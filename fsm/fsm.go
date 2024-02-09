package fsm

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"

	"github.com/ph-ngn/nanobox/cache"
	"github.com/ph-ngn/nanobox/telemetry"
)

const (
	RaftTimeOut         = 15 * time.Second
	ConfigChangeTimeOut = 0
	RetainSnapshotCount = 2
)

var (
	ErrNotRaftLeader        = errors.New("state machine is not Raft leader")
	ErrUnsupportedOperation = errors.New("event has unsupported operation")
)

// Event represents an event in the event log that will get replicated to Raft followers
type Event struct {
	Op     string    `json:"op,omitempty"`
	Key    string    `json:"key,omitempty"`
	Bvalue []byte    `json:"bvalue,omitempty"`
	Ivalue int64     `json:"ivalue,omitempty"`
	Expiry time.Time `json:"expiry,omitempty"`
}

// FiniteStateMachine is a wrapper around Store and manages replication with Raft consensus
type FiniteStateMachine struct {
	cache.Cache

	cap atomic.Int64

	raft *raft.Raft
}

type Config struct {
	Cache cache.Cache

	RaftBindAddr, RaftDir, FQDN, ID string

	BootstrapCluster bool
}

func (c *Config) validate() error {
	if c.Cache == nil {
		return errors.New("a cache implementation must be provided")
	}

	if c.RaftBindAddr == "" || c.RaftDir == "" || c.FQDN == "" || c.ID == "" {
		return errors.New("RaftAddr, RaftDir, RaftID must be provided")
	}

	return nil
}

func NewFSM(cfg Config) (fsm *FiniteStateMachine, err error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	raftCfg := raft.DefaultConfig()
	raftCfg.LocalID = raft.ServerID(cfg.ID)

	clusterAddr, err := net.ResolveTCPAddr("tcp", cfg.FQDN)
	if err != nil {
		return
	}
	transport, err := raft.NewTCPTransport(cfg.RaftBindAddr, clusterAddr, 4, 15*time.Second, os.Stderr)
	if err != nil {
		return
	}

	snapshots, err := raft.NewFileSnapshotStore(cfg.RaftDir, RetainSnapshotCount, os.Stderr)
	if err != nil {
		return
	}

	boltDB, err := raftboltdb.New(raftboltdb.Options{
		Path: filepath.Join(cfg.RaftDir, "raft.db"),
	})
	if err != nil {
		return
	}

	fsm = &FiniteStateMachine{
		Cache: cfg.Cache,
	}

	node, err := raft.NewRaft(raftCfg, fsm, boltDB, boltDB, snapshots, transport)
	if err != nil {
		return
	}

	if cfg.BootstrapCluster {
		clusterCfg := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      raftCfg.LocalID,
					Address: transport.LocalAddr(),
				},
			},
		}
		node.BootstrapCluster(clusterCfg)
	}

	fsm.raft = node
	return
}

func (fsm *FiniteStateMachine) DiscoverLeader() (raft.ServerAddress, raft.ServerID) {
	return fsm.raft.LeaderWithID()
}

func (fsm *FiniteStateMachine) Get(key string) (cache.Entry, bool) {
	if !fsm.isRaftLeader() {
		return fsm.Cache.Peek(key)
	}

	return fsm.Cache.Get(key)
}

func (fsm *FiniteStateMachine) Cap() int64 {
	if !fsm.isRaftLeader() {
		return fsm.cap.Load()
	}

	return fsm.Cache.Cap()
}

func (fsm *FiniteStateMachine) Set(key string, value []byte, ttl time.Duration) (bool, error) {
	if !fsm.isRaftLeader() {
		telemetry.Log().Errorf("Calling Set on follower")
		return false, ErrNotRaftLeader
	}

	event := Event{
		Op:     "set",
		Key:    key,
		Bvalue: value,
	}

	if ttl > 0 {
		event.Expiry = time.Now().Add(ttl)
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
		Op:     "update",
		Key:    key,
		Bvalue: value,
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

func (fsm *FiniteStateMachine) Resize(cap int64) error {
	if !fsm.isRaftLeader() {
		telemetry.Log().Errorf("Calling Resize on follower")
		return ErrNotRaftLeader
	}

	event := Event{
		Op:     "resize",
		Ivalue: cap,
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
		if !event.Expiry.IsZero() && event.Expiry.Before(time.Now()) {
			return nil
		}

		ttl := time.Duration(-1)
		if !event.Expiry.IsZero() {
			ttl = time.Until(event.Expiry)
		}

		return fsm.Cache.Set(event.Key, event.Bvalue, ttl)

	case "update":
		return fsm.Cache.Update(event.Key, event.Bvalue)

	case "delete":
		return fsm.Cache.Delete(event.Key)

	case "purge":
		fsm.Cache.Purge()
		return nil

	case "resize":
		cap := event.Ivalue
		if cap <= 0 {
			cap = -1
		}
		fsm.cap.Store(cap)
		// disable eviction on replicas, but still store the current cap to restore when become leader
		if fsm.isRaftLeader() {
			fsm.Cache.Resize(cap)
		}

		return nil

	default:
		return ErrUnsupportedOperation
	}
}

func (fsm *FiniteStateMachine) Join(nodeID, addr string) error {
	telemetry.Log().Infof("Received join request from node %s at %s", nodeID, addr)
	if !fsm.isRaftLeader() {
		telemetry.Log().Errorf("Calling Join on follower")
		return ErrNotRaftLeader
	}
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
	return &Snapshot{memory: fsm.Entries()}, nil
}

func (fsm *FiniteStateMachine) Restore(rc io.ReadCloser) error {
	snapshot := make([]cache.Entry, 0)
	if err := json.NewDecoder(rc).Decode(&snapshot); err != nil {
		return err
	}

	fsm.Cache.Recover(snapshot)
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
