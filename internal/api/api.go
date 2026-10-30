//go:generate mockgen -source=$GOFILE -destination=mocks_test.go -package=api
package api

import (
	"context"
	"errors"
	"strings"
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
	grpcapi.UnimplementedSecretsServer

	version  string
	provider contracts.Provider
	syncer   Syncer
}

// NewService creates Controller and Secrets services.
func NewService(version string, provider contracts.Provider, syncer Syncer) *Service {
	return &Service{
		version:  version,
		provider: provider,
		syncer:   syncer,
	}
}

// GetInfo returns process and provider metadata.
func (s *Service) GetInfo(context.Context, *grpcapi.GetInfoRequest) (*grpcapi.GetInfoResponse, error) {
	definition := s.provider.Definition()
	response := &grpcapi.GetInfoResponse{
		Version: s.version,
		Provider: &grpcapi.Provider{
			Name: definition.Name,
		},
	}
	if definition.Links.Doc != "" || definition.Links.Manager != "" {
		response.Provider.Links = &grpcapi.Links{
			Doc:     definition.Links.Doc,
			Manager: definition.Links.Manager,
		}
	}

	if lastSyncAt, ok := s.syncer.LastSyncAt(); ok {
		response.LastSyncAt = timestamppb.New(lastSyncAt)
	}
	if nextSyncAt, ok := s.syncer.NextSyncAt(); ok {
		response.NextSyncAt = timestamppb.New(nextSyncAt)
	}

	return response, nil
}

// Create validates and creates a secret without triggering synchronization.
func (s *Service) Create(ctx context.Context, req *grpcapi.CreateSecretRequest) (*grpcapi.CreateSecretResponse, error) {
	if req == nil || strings.TrimSpace(req.GetPath()) == "" {
		return nil, status.Error(codes.InvalidArgument, "path is required")
	}

	secret, err := s.provider.CreateSecret(ctx, contracts.CreateSecretRequest{
		Path:        req.GetPath(),
		Value:       req.GetValue(),
		Key:         req.GetKey(),
		Description: req.GetDescription(),
	})
	if err != nil {
		return nil, createSecretError(err)
	}

	response := &grpcapi.CreateSecretResponse{Path: secret.Path}
	if secret.WebURL != "" {
		response.ManagerUrl = &secret.WebURL
	}

	return response, nil
}

func createSecretError(err error) error {
	switch {
	case errors.Is(err, context.Canceled), status.Code(err) == codes.Canceled:
		return status.Error(codes.Canceled, "secret creation canceled")
	case errors.Is(err, context.DeadlineExceeded), status.Code(err) == codes.DeadlineExceeded:
		return status.Error(codes.DeadlineExceeded, "secret creation deadline exceeded")
	case errors.Is(err, contracts.ErrInvalidCreateRequest):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, contracts.ErrSecretAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, contracts.ErrCreateConflict):
		return status.Error(codes.Aborted, err.Error())
	case status.Code(err) == codes.NotFound:
		return status.Error(codes.FailedPrecondition, "required provider folder was not found")
	default:
		return status.Error(codes.Internal, "secret creation failed")
	}
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
