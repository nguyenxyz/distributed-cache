package nbox

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"

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

const PortPadding = 4000

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

func (s *nanoboxServer) DiscoverMaster(ctx context.Context, request *DiscoverMasterRequest) (*DiscoverMasterResponse, error) {
	fqdn, id := s.fsm.DiscoverLeader()
	padding := func(address string, increment int) string {
		parts := strings.Split(address, ":")
		port, _ := strconv.Atoi(parts[1])
		newPort := port + increment
		parts[1] = strconv.Itoa(newPort)
		return strings.Join(parts, ":")
	}

	return &DiscoverMasterResponse{
		FQDN: padding(string(fqdn), PortPadding),
		ID:   string(id),
	}, nil
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

	flag, err := s.fsm.Set(request.GetKey(), request.GetValue(), request.GetTtl().AsDuration())
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
		attribute.String("req.err", response.GetErrorCode().String()),
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
		attribute.String("req.err", response.GetErrorCode().String()),
	)

	return response, err
}

func (s *nanoboxServer) Delete(ctx context.Context, request *DeleteOrPurgeRequest) (*DeleteOrPurgeResponse, error) {
	span := trace.SpanFromContext(ctx)
	peer, _ := peer.FromContext(ctx)

	telemetry.Log().Infof("[DELETE] key: %s, from: %s", request.GetKey(), peer.Addr.String())

	flag, err := s.fsm.Delete(request.GetKey())
	response := &DeleteOrPurgeResponse{Flag: flag}
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
		attribute.String("req.err", response.GetErrorCode().String()),
	)

	return response, err
}

func (s *nanoboxServer) Purge(ctx context.Context, request *DeleteOrPurgeRequest) (*DeleteOrPurgeResponse, error) {
	span := trace.SpanFromContext(ctx)
	peer, _ := peer.FromContext(ctx)

	telemetry.Log().Infof("[PURGE] from: %s", peer.Addr.String())
	response := &DeleteOrPurgeResponse{}
	err := s.fsm.Purge()
	if err != nil {
		switch {
		case errors.Is(err, fsm.ErrNotRaftLeader):
			response.ErrorCode = ErrorCode_NOTMASTER

		default:
			response.ErrorCode = ErrorCode_INTERNAL
		}
	}

	span.SetAttributes(
		attribute.String("req.from", peer.Addr.String()),
		attribute.String("req.err", response.GetErrorCode().String()),
	)

	return response, err
}

func (s *nanoboxServer) Keys(ctx context.Context, request *KeysRequest) (*KeysResponse, error) {
	peer, _ := peer.FromContext(ctx)
	telemetry.Log().Infof("[KEYS] from: %s", peer.Addr.String())

	return &KeysResponse{Keys: s.fsm.Keys()}, nil
}

func (s *nanoboxServer) Entries(ctx context.Context, request *EntriesRequest) (*EntriesResponse, error) {
	peer, _ := peer.FromContext(ctx)
	telemetry.Log().Infof("[ENTRIES] from: %s", peer.Addr.String())

	entries := s.fsm.Entries()
	es := make([]*Entry, 0, len(entries))
	for _, entry := range entries {
		es = append(es, &Entry{
			Key:          entry.Key(),
			Value:        entry.Value(),
			LastUpdated:  timestamppb.New(entry.LastUpdated()),
			CreationTime: timestamppb.New(entry.CreationTime()),
			ExpiryTime:   timestamppb.New(entry.ExpiryTime()),
			Ttl:          durationpb.New(entry.TTL()),
		})
	}

	return &EntriesResponse{Entries: es}, nil
}

func (s *nanoboxServer) Size(ctx context.Context, request *SizeOrCapRequest) (*SizeOrCapResponse, error) {
	peer, _ := peer.FromContext(ctx)
	telemetry.Log().Infof("[SIZE] from: %s", peer.Addr.String())

	return &SizeOrCapResponse{Size: int64(s.fsm.Size())}, nil
}

func (s *nanoboxServer) Cap(ctx context.Context, request *SizeOrCapRequest) (*SizeOrCapResponse, error) {
	peer, _ := peer.FromContext(ctx)
	telemetry.Log().Infof("[CAP] from: %s", peer.Addr.String())

	return &SizeOrCapResponse{Size: int64(s.fsm.Cap())}, nil
}

func (s *nanoboxServer) Resize(ctx context.Context, request *ResizeRequest) (*ResizeResponse, error) {
	span := trace.SpanFromContext(ctx)
	peer, _ := peer.FromContext(ctx)

	telemetry.Log().Infof("[RESIZE] size: %d, from: %s", request.GetSize(), peer.Addr.String())

	err := s.fsm.Resize(request.GetSize())
	response := &ResizeResponse{}
	if err != nil {
		switch {
		case errors.Is(err, fsm.ErrNotRaftLeader):
			response.ErrorCode = ErrorCode_NOTMASTER

		default:
			response.ErrorCode = ErrorCode_INTERNAL
		}
	}

	span.SetAttributes(
		attribute.Int64("req.size", request.GetSize()),
		attribute.String("req.from", peer.Addr.String()),
		attribute.String("req.err", response.GetErrorCode().String()),
	)

	return response, err
}

func (s *nanoboxServer) Join(ctx context.Context, request *JoinRequest) (*JoinResponse, error) {
	span := trace.SpanFromContext(ctx)
	peer, _ := peer.FromContext(ctx)

	telemetry.Log().Infof("[JOIN] from: %s", peer.Addr.String())

	// wait for the peers to finish setting up
	time.Sleep(10 * time.Second)

	err := s.fsm.Join(request.GetFQDN(), request.GetID())
	response := &JoinResponse{}
	if err != nil {
		switch {
		case errors.Is(err, fsm.ErrNotRaftLeader):
			response.ErrorCode = ErrorCode_NOTMASTER

		default:
			response.ErrorCode = ErrorCode_INTERNAL
		}
	}

	span.SetAttributes(
		attribute.String("req.from", peer.Addr.String()),
		attribute.String("req.err", response.GetErrorCode().String()),
	)

	return response, err
}
