//go:generate mockgen -source=$GOFILE -destination=mocks_test.go -package=api
package api

import (
	"context"
	"errors"
	"time"

	"github.com/swarm-deploy/cloud-secrets/internal/application/cs"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
	cloudsecretssync "github.com/swarm-deploy/cloud-secrets/internal/sync"
	grpcapi "github.com/swarm-deploy/cloud-secrets/pkg/grpc-api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const grpcSyncTrigger = "grpc"

// Syncer runs synchronization and exposes its last successful completion time.
type Syncer interface {
	// Sync runs synchronization for the provided trigger.
	Sync(ctx context.Context, trigger string) (cloudsecretssync.Result, error)
	// LastSyncAt returns the latest successful synchronization known by the process.
	LastSyncAt() (time.Time, bool)
	// NextSyncAt returns the next scheduled interval synchronization time.
	NextSyncAt() (time.Time, bool)
}

// Service implements the cloud-secrets integration API.
type Service struct {
	grpcapi.UnimplementedControllerServer

	version  string
	provider contracts.ProviderDefinition
	syncer   Syncer
}

// NewService creates a Controller service.
func NewService(version string, provider contracts.ProviderDefinition, syncer Syncer) *Service {
	return &Service{
		version:  version,
		provider: provider,
		syncer:   syncer,
	}
}

// GetInfo returns process and provider metadata.
func (s *Service) GetInfo(context.Context, *grpcapi.GetInfoRequest) (*grpcapi.GetInfoResponse, error) {
	response := &grpcapi.GetInfoResponse{
		Version: s.version,
		Provider: &grpcapi.Provider{
			Name: s.provider.Name,
			Link: s.provider.URL,
		},
	}

	if lastSyncAt, ok := s.syncer.LastSyncAt(); ok {
		response.LastSyncAt = timestamppb.New(lastSyncAt)
	}
	if nextSyncAt, ok := s.syncer.NextSyncAt(); ok {
		response.NextSyncAt = timestamppb.New(nextSyncAt)
	}

	return response, nil
}

// Sync triggers the normal synchronization flow on demand.
func (s *Service) Sync(ctx context.Context, _ *grpcapi.SyncRequest) (*grpcapi.SyncResponse, error) {
	result, err := s.syncer.Sync(ctx, grpcSyncTrigger)
	if err != nil {
		switch {
		case errors.Is(err, cs.ErrSyncInProgress):
			return nil, status.Error(codes.Aborted, err.Error())
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return nil, status.FromContextError(err).Err()
		default:
			return nil, status.Error(codes.Internal, "synchronization failed")
		}
	}

	return &grpcapi.SyncResponse{
		Created:   uint32(result.Created),        //nolint:gosec // counters are non-negative and bounded by Docker resources
		Updated:   uint32(result.Updated),        //nolint:gosec // counters are non-negative and bounded by Docker resources
		Removed:   uint32(result.RemovedSecrets), //nolint:gosec // counters are non-negative and bounded by Docker resources
		Unchanged: uint32(result.Skipped),        //nolint:gosec // counters are non-negative and bounded by Docker resources
	}, nil
}
