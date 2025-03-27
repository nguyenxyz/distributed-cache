package apiserver

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/phonghmnguyen/ke0/apiserver/proto"
	"github.com/phonghmnguyen/ke0/raft"
	"github.com/phonghmnguyen/ke0/telemetry"
)

var _ proto.Ke0APIServer = (*server)(nil)

type server struct {
	proto.UnimplementedKe0APIServer

	raftIns *raft.Instance

	grpc *grpc.Server
}

func NewServer(ctx context.Context, raftIns *raft.Instance, grpc *grpc.Server) *server {
	return &server{
		raftIns: raftIns,
		grpc:    grpc,
	}
}

func (s *server) ListenAndServe(addr string) error {
	telemetry.Log().Infof("Starting API Server on %s", addr)
	proto.RegisterKe0APIServer(s.grpc, s)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	return s.grpc.Serve(lis)
}

func (s *server) GracefulStop() {
	s.grpc.GracefulStop()
}

func (s *server) DiscoverMaster(ctx context.Context, request *proto.DiscoverMasterRequest) (*proto.DiscoverMasterResponse, error) {
	fqdn, id := s.raftIns.DiscoverLeader()
	redirect := func(addr string) string {
		parts := strings.Split(addr, ":")
		parts[1] = os.Getenv("gRPC_ADDRESS")
		return strings.Join(parts, ":")
	}

	return &proto.DiscoverMasterResponse{
		FQDN: redirect(string(fqdn)),
		ID:   string(id),
	}, nil
}

func (s *server) Get(ctx context.Context, request *proto.GetOrPeakRequest) (*proto.GetOrPeakResponse, error) {
	peer, _ := peer.FromContext(ctx)
	telemetry.Log().Infof("[GET] key: %s, from: %s", request.GetKey(), peer.Addr.String())

	entry, ok := s.raftIns.Get(request.GetKey())
	response := &proto.GetOrPeakResponse{Flag: ok}
	if ok {
		response.Entry = &proto.Entry{
			Key:          entry.Key(),
			Value:        entry.Value(),
			LastUpdated:  timestamppb.New(entry.LastUpdated()),
			CreationTime: timestamppb.New(entry.CreationTime()),
			ExpiryTime:   timestamppb.New(entry.ExpiryTime()),
			Ttl:          durationpb.New(entry.TTL()),
		}
	}

	return response, nil
}

func (s *server) Peek(ctx context.Context, request *proto.GetOrPeakRequest) (*proto.GetOrPeakResponse, error) {
	peer, _ := peer.FromContext(ctx)
	telemetry.Log().Infof("[PEEK] key: %s, from: %s", request.GetKey(), peer.Addr.String())

	entry, ok := s.raftIns.Peek(request.GetKey())
	response := &proto.GetOrPeakResponse{Flag: ok}
	if ok {
		response.Entry = &proto.Entry{
			Key:          entry.Key(),
			Value:        entry.Value(),
			LastUpdated:  timestamppb.New(entry.LastUpdated()),
			CreationTime: timestamppb.New(entry.CreationTime()),
			ExpiryTime:   timestamppb.New(entry.ExpiryTime()),
			Ttl:          durationpb.New(entry.TTL()),
		}
	}

	return response, nil
}

func (s *server) Set(ctx context.Context, request *proto.SetOrUpdateRequest) (*proto.SetOrUpdateResponse, error) {
	span := trace.SpanFromContext(ctx)
	peer, _ := peer.FromContext(ctx)

	telemetry.Log().Infof("[SET] key: %s, from: %s", request.GetKey(), peer.Addr.String())

	flag, err := s.raftIns.Set(request.GetKey(), request.GetValue(), request.GetTtl().AsDuration())
	response := &proto.SetOrUpdateResponse{Flag: flag}
	if err != nil {
		switch {
		case errors.Is(err, raft.ErrNotRaftLeader):
			response.ErrorCode = proto.ErrorCode_NOTMASTER

		default:
			response.ErrorCode = proto.ErrorCode_INTERNAL
		}
	}

	span.SetAttributes(
		attribute.String("req.key", request.GetKey()),
		attribute.String("req.from", peer.Addr.String()),
		attribute.String("req.err", response.GetErrorCode().String()),
	)

	return response, err
}

func (s *server) Update(ctx context.Context, request *proto.SetOrUpdateRequest) (*proto.SetOrUpdateResponse, error) {
	span := trace.SpanFromContext(ctx)
	peer, _ := peer.FromContext(ctx)

	telemetry.Log().Infof("[UPDATE] key: %s, from: %s", request.GetKey(), peer.Addr.String())

	flag, err := s.raftIns.Update(request.GetKey(), request.GetValue())
	response := &proto.SetOrUpdateResponse{Flag: flag}
	if err != nil {
		switch {
		case errors.Is(err, raft.ErrNotRaftLeader):
			response.ErrorCode = proto.ErrorCode_NOTMASTER

		default:
			response.ErrorCode = proto.ErrorCode_INTERNAL
		}
	}

	span.SetAttributes(
		attribute.String("req.key", request.GetKey()),
		attribute.String("req.from", peer.Addr.String()),
		attribute.String("req.err", response.GetErrorCode().String()),
	)

	return response, err
}

func (s *server) Delete(ctx context.Context, request *proto.DeleteOrPurgeRequest) (*proto.DeleteOrPurgeResponse, error) {
	span := trace.SpanFromContext(ctx)
	peer, _ := peer.FromContext(ctx)

	telemetry.Log().Infof("[DELETE] key: %s, from: %s", request.GetKey(), peer.Addr.String())

	flag, err := s.raftIns.Delete(request.GetKey())
	response := &proto.DeleteOrPurgeResponse{Flag: flag}
	if err != nil {
		switch {
		case errors.Is(err, raft.ErrNotRaftLeader):
			response.ErrorCode = proto.ErrorCode_NOTMASTER

		default:
			response.ErrorCode = proto.ErrorCode_INTERNAL
		}
	}

	span.SetAttributes(
		attribute.String("req.key", request.GetKey()),
		attribute.String("req.from", peer.Addr.String()),
		attribute.String("req.err", response.GetErrorCode().String()),
	)

	return response, err
}

func (s *server) Purge(ctx context.Context, request *proto.DeleteOrPurgeRequest) (*proto.DeleteOrPurgeResponse, error) {
	span := trace.SpanFromContext(ctx)
	peer, _ := peer.FromContext(ctx)

	telemetry.Log().Infof("[PURGE] from: %s", peer.Addr.String())
	response := &proto.DeleteOrPurgeResponse{}
	err := s.raftIns.Purge()
	if err != nil {
		switch {
		case errors.Is(err, raft.ErrNotRaftLeader):
			response.ErrorCode = proto.ErrorCode_NOTMASTER

		default:
			response.ErrorCode = proto.ErrorCode_INTERNAL
		}
	}

	span.SetAttributes(
		attribute.String("req.from", peer.Addr.String()),
		attribute.String("req.err", response.GetErrorCode().String()),
	)

	return response, err
}

func (s *server) Keys(ctx context.Context, request *proto.KeysRequest) (*proto.KeysResponse, error) {
	peer, _ := peer.FromContext(ctx)
	telemetry.Log().Infof("[KEYS] from: %s", peer.Addr.String())

	return &proto.KeysResponse{Keys: s.raftIns.Keys()}, nil
}

func (s *server) Entries(ctx context.Context, request *proto.EntriesRequest) (*proto.EntriesResponse, error) {
	peer, _ := peer.FromContext(ctx)
	telemetry.Log().Infof("[ENTRIES] from: %s", peer.Addr.String())

	entries := s.raftIns.Entries()
	es := make([]*proto.Entry, 0, len(entries))
	for _, entry := range entries {
		es = append(es, &proto.Entry{
			Key:          entry.Key(),
			Value:        entry.Value(),
			LastUpdated:  timestamppb.New(entry.LastUpdated()),
			CreationTime: timestamppb.New(entry.CreationTime()),
			ExpiryTime:   timestamppb.New(entry.ExpiryTime()),
			Ttl:          durationpb.New(entry.TTL()),
		})
	}

	return &proto.EntriesResponse{Entries: es}, nil
}

func (s *server) Size(ctx context.Context, request *proto.SizeOrCapRequest) (*proto.SizeOrCapResponse, error) {
	peer, _ := peer.FromContext(ctx)
	telemetry.Log().Infof("[SIZE] from: %s", peer.Addr.String())

	return &proto.SizeOrCapResponse{Size: int64(s.raftIns.Size())}, nil
}

func (s *server) Cap(ctx context.Context, request *proto.SizeOrCapRequest) (*proto.SizeOrCapResponse, error) {
	peer, _ := peer.FromContext(ctx)
	telemetry.Log().Infof("[CAP] from: %s", peer.Addr.String())

	return &proto.SizeOrCapResponse{Size: int64(s.raftIns.Cap())}, nil
}

func (s *server) Resize(ctx context.Context, request *proto.ResizeRequest) (*proto.ResizeResponse, error) {
	span := trace.SpanFromContext(ctx)
	peer, _ := peer.FromContext(ctx)

	telemetry.Log().Infof("[RESIZE] size: %d, from: %s", request.GetSize(), peer.Addr.String())

	err := s.raftIns.Resize(request.GetSize())
	response := &proto.ResizeResponse{}
	if err != nil {
		switch {
		case errors.Is(err, raft.ErrNotRaftLeader):
			response.ErrorCode = proto.ErrorCode_NOTMASTER

		default:
			response.ErrorCode = proto.ErrorCode_INTERNAL
		}
	}

	span.SetAttributes(
		attribute.Int64("req.size", request.GetSize()),
		attribute.String("req.from", peer.Addr.String()),
		attribute.String("req.err", response.GetErrorCode().String()),
	)

	return response, err
}

func (s *server) Join(ctx context.Context, request *proto.JoinRequest) (*proto.JoinResponse, error) {
	span := trace.SpanFromContext(ctx)
	peer, _ := peer.FromContext(ctx)

	telemetry.Log().Infof("[JOIN] from: %s", peer.Addr.String())

	// wait for the peers to finish setting up
	time.Sleep(10 * time.Second)

	err := s.raftIns.Join(request.GetFQDN(), request.GetID())
	response := &proto.JoinResponse{}
	if err != nil {
		switch {
		case errors.Is(err, raft.ErrNotRaftLeader):
			response.ErrorCode = proto.ErrorCode_NOTMASTER

		default:
			response.ErrorCode = proto.ErrorCode_INTERNAL
		}
	}

	span.SetAttributes(
		attribute.String("req.from", peer.Addr.String()),
		attribute.String("req.err", response.GetErrorCode().String()),
	)

	return response, err
}
