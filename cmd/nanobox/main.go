package main

import (
	"context"
	"fmt"

	"github.com/ph-ngn/nanobox/cache"
	"github.com/ph-ngn/nanobox/fsm"
	"github.com/ph-ngn/nanobox/nbox"
	"github.com/ph-ngn/nanobox/telemetry"
	"google.golang.org/grpc"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	telemetry.Init(ctx)
	lru := cache.NewLRU(ctx, cache.WithCapacity(1000))
	fsm, err := fsm.NewFSM(fsm.Config{
		Cache:            lru,
		RaftBindAddr:     "localhost:4000",
		RaftDir:          "./node1",
		FQDN:             "localhost:4000", //"nanobox-1.nanobox.default.svc.cluster.local:4001"
		ID:               "1",
		BootstrapCluster: true,
	})

	if err != nil {
		fmt.Println("hello")
		panic(err.Error())
	}

	nbox := nbox.NewNanoboxServer(ctx, fsm, grpc.NewServer())
	if err := nbox.ListenAndServe("0.0.0.0:4001"); err != nil {
		panic(err.Error())
	}
}
