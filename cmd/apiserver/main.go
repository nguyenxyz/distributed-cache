package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	"github.com/phonghmnguyen/ke0/apiserver"
	"github.com/phonghmnguyen/ke0/apiserver/proto"
	"github.com/phonghmnguyen/ke0/cache"
	"github.com/phonghmnguyen/ke0/raft"
	"github.com/phonghmnguyen/ke0/telemetry"
)

var (
	grpcAddr, RaftBindAddr, RaftDir, FQDN, ID string
	BootstrapCluster                          bool
	Capacity                                  int
)

func init() {
	//"nanobox-0.nanobox.default.svc.cluster.local:8000"
	flag.StringVar(&grpcAddr, "grpc", "0.0.0.0:8000", "gRPC address")
	flag.StringVar(&RaftBindAddr, "raft", "0.0.0.0:4000", "Raft local bind address")
	flag.StringVar(&RaftDir, "raftdir", "./raftdir", "Raft directory for storing logs and snapshots")
	//"nanobox-0.nanobox.default.svc.cluster.local:4000"
	var (
		ordinal     = os.Getenv("ORDINAL_NUMBER")
		podName     = os.Getenv("POD_NAME")
		namespace   = os.Getenv("NAMESPACE")
		serviceName = os.Getenv("SERVICE_NAME")
	)

	fqdn := "localhost:4000"
	if podName != "" && namespace != "" && serviceName != "" {
		fqdn = fmt.Sprintf("%s.%s.%s.svc.cluster.local:4000", podName, serviceName, namespace)
	}

	flag.StringVar(&ID, "id", ordinal, "Raft node ID")
	flag.StringVar(&FQDN, "fqdn", fqdn, "Raft cluster address")
	flag.BoolVar(&BootstrapCluster, "bootstrap", ordinal == "0", "Bootstrap cluster flag")
	flag.IntVar(&Capacity, "cap", 1000, "Capacity of the cache")
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	flag.Parse()
	telemetry.Init(ctx, FQDN, ID, fmt.Sprintf("%s.log", FQDN))
	var raftInstance *raft.Instance
	// sync evictions to replicas
	onEvict := func(key string) {
		telemetry.Log().Infof("Syncing eviction of key %s from master", key)
		go raftInstance.Delete(key)
	}

	lru := cache.NewMemoryCache(ctx, cache.WithCapacity(Capacity), cache.WithEvictionCallback(onEvict), cache.WithEvictionPolicy(cache.NewLRUPolicy()))
	raftInstance, err := raft.NewInstance(raft.Config{
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
		if err := join(ctx, "nanobox-0.nanobox.default.svc.cluster.local:8000", FQDN, ID); err != nil {
			telemetry.Log().Infof("Maybe already joined if pod restarts")
		}
	}

	apiServer := apiserver.NewServer(ctx, raftInstance, grpc.NewServer(grpc.KeepaliveParams(keepalive.ServerParameters{
		MaxConnectionAge:      30 * time.Second,
		MaxConnectionAgeGrace: 10 * time.Second,
	})))

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	serverErrChan := make(chan error, 1)
	go func() {
		if err := apiServer.ListenAndServe(grpcAddr); err != nil {
			serverErrChan <- err
		}
	}()

	select {
	case sig := <-signalChan:
		telemetry.Log().Infof("Received signal: %v, initiating graceful shutdown", sig)
	case err := <-serverErrChan:
		telemetry.Log().Errorf("API Server error: %v", err)
	}

	apiServer.GracefulStop()
	telemetry.Log().Infof("API Server shutdown complete")
}

func join(ctx context.Context, leaderAddr, FQDN, ID string) error {
	conn, err := grpc.DialContext(ctx, leaderAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = proto.NewKe0APIClient(conn).Join(ctx, &proto.JoinRequest{
		FQDN: FQDN,
		ID:   ID,
	})
	if err != nil {
		return err
	}

	return nil

}
