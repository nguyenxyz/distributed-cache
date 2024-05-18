package main

import (
	"context"

	"github.com/ph-ngn/nanobox/apiserver"
	"github.com/ph-ngn/nanobox/nbox"
	"github.com/ph-ngn/nanobox/telemetry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	telemetry.Init(ctx)

	conn, err := grpc.DialContext(ctx, "localhost:8000",
		grpc.WithDefaultServiceConfig(`{"loadBalancingConfig": [{"round_robin":{}}]}`),
		grpc.WithTransportCredentials(insecure.NewCredentials()))

	if err != nil {
		panic(err.Error())
	}

	defer conn.Close()

	cc := apiserver.NewClusterController(ctx, nbox.NewNanoboxClient(conn))
	server, err := apiserver.NewServer(cc)
	if err != nil {
		panic(err.Error())
	}

	server.ListenAndServe("0.0.0.0:9000")

}
