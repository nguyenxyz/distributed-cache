package raft

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	hraft "github.com/hashicorp/raft"
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

// RaftInstance is a wrapper around cache and manages replication with Raft consensus
type RaftInstance struct {
	cache.Cache

	cap atomic.Int64

	raft *hraft.Raft
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
		return errors.New("RaftAddr, RaftDir, RaftFQDN, RaftID must be provided")
	}

	return nil
}

func NewRaftInstance(cfg Config) (ri *RaftInstance, err error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	raftCfg := hraft.DefaultConfig()
	raftCfg.LocalID = hraft.ServerID(cfg.ID)

	clusterAddr, err := net.ResolveTCPAddr("tcp", cfg.FQDN)
	if err != nil {
		return
	}
	transport, err := hraft.NewTCPTransport(cfg.RaftBindAddr, clusterAddr, 4, 15*time.Second, os.Stderr)
	if err != nil {
		return
	}

	snapshots, err := hraft.NewFileSnapshotStore(cfg.RaftDir, RetainSnapshotCount, os.Stderr)
	if err != nil {
		return
	}

	boltDB, err := raftboltdb.New(raftboltdb.Options{
		Path: filepath.Join(cfg.RaftDir, "raft.db"),
	})
	if err != nil {
		return
	}

	ri = &RaftInstance{
		Cache: cfg.Cache,
	}

	node, err := hraft.NewRaft(raftCfg, ri, boltDB, boltDB, snapshots, transport)
	if err != nil {
		return
	}

	if cfg.BootstrapCluster {
		clusterCfg := hraft.Configuration{
			Servers: []hraft.Server{
				{
					ID:      raftCfg.LocalID,
					Address: transport.LocalAddr(),
				},
			},
		}
		node.BootstrapCluster(clusterCfg)
	}

	ri.raft = node
	return
}

func (ri *RaftInstance) DiscoverLeader() (hraft.ServerAddress, hraft.ServerID) {
	return ri.raft.LeaderWithID()
}

func (ri *RaftInstance) Cap() int64 {
	if !ri.isRaftLeader() {
		return ri.cap.Load()
	}

	return ri.Cache.Cap()
}

func (ri *RaftInstance) Set(key string, value []byte, ttl time.Duration) (bool, error) {
	if !ri.isRaftLeader() {
		telemetry.Log().Warnf("Calling Set on follower")
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

	res, err := ri.replicateAndApplyOnQuorum(event)
	if err == nil {
		telemetry.Log().Debugf("Succesfully replicate and apply event: %+v", event)
		return res.(bool), nil
	}

	return false, err
}

func (ri *RaftInstance) Update(key string, value []byte) (bool, error) {
	if !ri.isRaftLeader() {
		telemetry.Log().Errorf("Calling Update on follower")
		return false, ErrNotRaftLeader
	}

	event := Event{
		Op:     "update",
		Key:    key,
		Bvalue: value,
	}

	res, err := ri.replicateAndApplyOnQuorum(event)
	if err == nil {
		telemetry.Log().Debugf("Succesfully replicate and apply event: %+v", event)
		return res.(bool), nil
	}

	return false, err
}

func (ri *RaftInstance) Delete(key string) (bool, error) {
	if !ri.isRaftLeader() {
		telemetry.Log().Errorf("Calling Delete on follower")
		return false, ErrNotRaftLeader
	}

	event := Event{
		Op:  "delete",
		Key: key,
	}

	res, err := ri.replicateAndApplyOnQuorum(event)
	if err == nil {
		telemetry.Log().Debugf("Succesfully replicate and apply event: %+v", event)
		return res.(bool), nil
	}

	return false, err
}

func (ri *RaftInstance) Purge() error {
	if !ri.isRaftLeader() {
		telemetry.Log().Errorf("Calling Purge on follower")
		return ErrNotRaftLeader
	}

	event := Event{
		Op: "purge",
	}

	_, err := ri.replicateAndApplyOnQuorum(event)
	if err == nil {
		telemetry.Log().Debugf("Succesfully replicate and apply event: %+v", event)
	}

	return err
}

func (ri *RaftInstance) Resize(cap int64) error {
	if !ri.isRaftLeader() {
		telemetry.Log().Errorf("Calling Resize on follower")
		return ErrNotRaftLeader
	}

	event := Event{
		Op:     "resize",
		Ivalue: cap,
	}

	_, err := ri.replicateAndApplyOnQuorum(event)
	if err == nil {
		telemetry.Log().Debugf("Succesfully replicate and apply event: %+v", event)
	}

	return err
}

// Apply applies an event from the log to the finite state machine and is called once a log entry is committed by a quorum of the cluster
func (ri *RaftInstance) Apply(l *hraft.Log) interface{} {
	var event Event
	if err := json.Unmarshal(l.Data, &event); err != nil {
		// need to exit and recover here so the ri get a chance to reapply the event
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

		return ri.Cache.Set(event.Key, event.Bvalue, ttl)

	case "update":
		return ri.Cache.Update(event.Key, event.Bvalue)

	case "delete":
		return ri.Cache.Delete(event.Key)

	case "purge":
		ri.Cache.Purge()
		return nil

	case "resize":
		cap := event.Ivalue
		if cap <= 0 {
			cap = -1
		}
		ri.cap.Store(cap)
		// disable eviction on replicas, but still store the current cap to restore when become leader
		if ri.isRaftLeader() {
			ri.Cache.Resize(cap)
		}

		return nil

	default:
		return ErrUnsupportedOperation
	}
}

func (ri *RaftInstance) Join(addr, nodeID string) error {
	telemetry.Log().Infof("Received join request from node %s at %s", nodeID, addr)
	if !ri.isRaftLeader() {
		telemetry.Log().Errorf("Calling Join on follower")
		return ErrNotRaftLeader
	}
	configFuture := ri.raft.GetConfiguration()
	if err := configFuture.Error(); err != nil {
		telemetry.Log().Errorf("Failed to get raft configuration: %v", err)
		return err
	}

	for _, srv := range configFuture.Configuration().Servers {
		if srv.ID == hraft.ServerID(nodeID) || srv.Address == hraft.ServerAddress(addr) {
			if srv.ID == hraft.ServerID(nodeID) && srv.Address == hraft.ServerAddress(addr) {
				telemetry.Log().Infof("node %s at %s is already a member", nodeID, addr)
				return nil
			}

			future := ri.raft.RemoveServer(srv.ID, 0, ConfigChangeTimeOut)
			if err := future.Error(); err != nil {
				telemetry.Log().Errorf("Failed to remove existing node %s at %s: %v", nodeID, addr, err)
				return err
			}
		}
	}

	future := ri.raft.AddVoter(hraft.ServerID(nodeID), hraft.ServerAddress(addr), 0, ConfigChangeTimeOut)
	if err := future.Error(); err != nil {
		telemetry.Log().Errorf("Failed to join node %s at %s: %v", nodeID, addr, err)
		return err
	}

	telemetry.Log().Infof("Successfully join node %s at %s", nodeID, addr)
	return nil
}

func (ri *RaftInstance) Snapshot() (hraft.FSMSnapshot, error) {
	return &Snapshot{memory: ri.Entries()}, nil
}

func (ri *RaftInstance) Restore(rc io.ReadCloser) error {
	snapshot := make([]cache.Entry, 0)
	if err := json.NewDecoder(rc).Decode(&snapshot); err != nil {
		return err
	}

	ri.Cache.Recover(snapshot)
	return nil
}

// replicateAndApplyOnQuorum replicates an event to followers and applies the event to the ri if it is commited by a quorum of the cluster
func (ri *RaftInstance) replicateAndApplyOnQuorum(event Event) (interface{}, error) {
	b, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}

	future := ri.raft.Apply(b, RaftTimeOut)
	if err := future.Error(); err != nil {
		telemetry.Log().Errorf("Encountered an error during Raft operation: %v", err)
		return nil, err
	}

	return future.Response(), nil
}

func (ri *RaftInstance) isRaftLeader() bool {
	return ri.raft.State() == hraft.Leader
}
