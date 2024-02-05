package nbox

import (
	"context"
	"errors"
	"net"

	"github.com/ph-ngn/nanobox/fsm"
	"github.com/ph-ngn/nanobox/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var _ NanoboxServer = (*nanoboxServer)(nil)

type nanoboxServer struct {
	UnimplementedNanoboxServer

	fsm *fsm.FiniteStateMachine

	grpc *grpc.Server
}

func NewNanoboxServer(ctx context.Context, fsm *fsm.FiniteStateMachine, grpc *grpc.Server) *nanoboxServer {
	return &nanoboxServer{
		fsm:  fsm,
		grpc: grpc,
	}
}

func (s *nanoboxServer) ListenAndServe(addr string) error {
	telemetry.Log().Infof("Starting Nanobox on %s", addr)
	RegisterNanoboxServer(s.grpc, s)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	return s.grpc.Serve(lis)
}

func (s *nanoboxServer) DiscoverMaster(ctx context.Context, request *MasterInfoRequest) (*MasterInfoResponse, error) {
	return &MasterInfoResponse{}, nil
}

func (s *nanoboxServer) Get(ctx context.Context, request *GetOrPeakRequest) (*GetOrPeakResponse, error) {
	peer, _ := peer.FromContext(ctx)
	telemetry.Log().Infof("[GET] key: %s, from: %s", request.GetKey(), peer.Addr.String())

	entry, ok := s.fsm.Get(request.GetKey())
	response := &GetOrPeakResponse{Flag: ok}
	if ok {
		response.Entry = &Entry{
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

func (s *nanoboxServer) Peek(ctx context.Context, request *GetOrPeakRequest) (*GetOrPeakResponse, error) {
	peer, _ := peer.FromContext(ctx)
	telemetry.Log().Infof("[PEEK] key: %s, from: %s", request.GetKey(), peer.Addr.String())

	entry, ok := s.fsm.Peek(request.GetKey())
	response := &GetOrPeakResponse{Flag: ok}
	if ok {
		response.Entry = &Entry{
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

func (s *nanoboxServer) Set(ctx context.Context, request *SetOrUpdateRequest) (*SetOrUpdateResponse, error) {
	span := trace.SpanFromContext(ctx)
	peer, _ := peer.FromContext(ctx)

	telemetry.Log().Infof("[SET] key: %s, from: %s", request.GetKey(), peer.Addr.String())

	flag, err := s.fsm.Set(request.GetKey(), request.GetValue())
	response := &SetOrUpdateResponse{Flag: flag}
	if err != nil {
		switch {
		case errors.Is(err, fsm.ErrNotRaftLeader):
			response.ErrorCode = ErrorCode_NOTMASTER

		default:
			response.ErrorCode = ErrorCode_INTERNAL
		}
	}

	span.SetAttributes(
		attribute.String("req.key", request.GetKey()),
		attribute.String("req.from", peer.Addr.String()),
		attribute.String("req.errCode", response.GetErrorCode().String()),
	)

	return response, err
}

func (s *nanoboxServer) Update(ctx context.Context, request *SetOrUpdateRequest) (*SetOrUpdateResponse, error) {
	span := trace.SpanFromContext(ctx)
	peer, _ := peer.FromContext(ctx)

	telemetry.Log().Infof("[UPDATE] key: %s, from: %s", request.GetKey(), peer.Addr.String())

	flag, err := s.fsm.Update(request.GetKey(), request.GetValue())
	response := &SetOrUpdateResponse{Flag: flag}
	if err != nil {
		switch {
		case errors.Is(err, fsm.ErrNotRaftLeader):
			response.ErrorCode = ErrorCode_NOTMASTER

		default:
			response.ErrorCode = ErrorCode_INTERNAL
		}
	}

	span.SetAttributes(
		attribute.String("req.key", request.GetKey()),
		attribute.String("req.from", peer.Addr.String()),
		attribute.String("req.errCode", response.GetErrorCode().String()),
	)

	return response, err
}
