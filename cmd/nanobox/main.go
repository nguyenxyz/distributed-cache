package main

import (
	"context"
	"flag"

	"github.com/ph-ngn/nanobox/cache"
	"github.com/ph-ngn/nanobox/fsm"
	"github.com/ph-ngn/nanobox/nbox"
	"github.com/ph-ngn/nanobox/telemetry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	grpcAddr, RaftBindAddr, RaftDir, FQDN, ID string
	BootstrapCluster                          bool
	Capacity                                  int64
)

func init() {
	//"nanobox-0.nanobox.default.svc.cluster.local:8000"
	flag.StringVar(&grpcAddr, "grpc", "localhost:8000", "gRPC address")
	flag.StringVar(&RaftBindAddr, "raft", "localhost:4000", "Raft local bind address")
	flag.StringVar(&RaftDir, "raftdir", "./raftdir", "Raft directory for storing logs and snapshots")
	//"nanobox-0.nanobox.default.svc.cluster.local:4000"
	flag.StringVar(&FQDN, "fqdn", "localhost:4000", "Raft cluster address")
	flag.StringVar(&ID, "id", "0", "Raft node ID")
	flag.BoolVar(&BootstrapCluster, "bootstrap", false, "Bootstrap cluster flag")
	flag.Int64Var(&Capacity, "cap", 1000, "Capacity of the cache")
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	flag.Parse()

	telemetry.Init(ctx)

	var sm *fsm.FiniteStateMachine
	// sync evictions to replicas
	cb := func(key string, value []byte) {
		go sm.Delete(key)
	}

	lru := cache.NewLRU(ctx, cache.WithCapacity(Capacity), cache.WithEvictionCallback(cb))
	sm, err := fsm.NewFSM(fsm.Config{
		Cache:            lru,
		RaftBindAddr:     RaftBindAddr,
		RaftDir:          RaftDir,
		FQDN:             FQDN, //"nanobox-1.nanobox.default.svc.cluster.local:4000"
		ID:               ID,
		BootstrapCluster: BootstrapCluster,
	})

	if err != nil {
		panic(err.Error())
	}

	if !BootstrapCluster {
		//"nanobox-0.nanobox.default.svc.cluster.local:8000"
		if err := join(ctx, "localhost:8000", FQDN, ID); err != nil {
			telemetry.Log().Infof("Maybe already joined if pod restarts")
		}
	}

	nbox := nbox.NewNanoboxServer(ctx, sm, grpc.NewServer())
	if err := nbox.ListenAndServe(grpcAddr); err != nil {
		panic(err.Error())
	}

}

func join(ctx context.Context, leaderAddr, FQDN, ID string) error {
	conn, err := grpc.DialContext(ctx, leaderAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = nbox.NewNanoboxClient(conn).Join(ctx, &nbox.JoinRequest{
		FQDN: FQDN,
		ID:   ID,
	})
	if err != nil {
		return err
	}

	return nil

}
