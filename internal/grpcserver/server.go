package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/swarm-deploy/cloud-secrets/internal/api"
	grpcapi "github.com/swarm-deploy/cloud-secrets/pkg/grpc-api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	grpc_health_v1 "google.golang.org/grpc/health/grpc_health_v1"
)

// Server hosts the optional integration gRPC API.
type Server struct {
	address      string
	grpcServer   *grpc.Server
	healthServer *health.Server
}

// New creates the integration gRPC server.
func New(address string, controllerService *api.Service) *Server {
	grpcServer := grpc.NewServer()
	healthServer := health.NewServer()

	grpcapi.RegisterControllerServer(grpcServer, controllerService)
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)

	return &Server{
		address:      address,
		grpcServer:   grpcServer,
		healthServer: healthServer,
	}
}

// Run binds to the configured address and serves requests until stopped.
func (s *Server) Run(ctx context.Context) error {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.address)
	if err != nil {
		return fmt.Errorf("listen on gRPC address %q: %w", s.address, err)
	}

	return s.serve(listener)
}

func (s *Server) serve(listener net.Listener) error {
	s.healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	s.healthServer.SetServingStatus(
		grpcapi.Controller_ServiceDesc.ServiceName,
		grpc_health_v1.HealthCheckResponse_SERVING,
	)

	if err := s.grpcServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return fmt.Errorf("serve gRPC API: %w", err)
	}

	return nil
}

// Stop gracefully stops accepting gRPC requests.
func (s *Server) Stop(ctx context.Context) error {
	s.healthServer.Shutdown()

	stopped := make(chan struct{})
	go func() {
		s.grpcServer.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
		return nil
	case <-ctx.Done():
		s.grpcServer.Stop()
		return fmt.Errorf("gracefully stop gRPC server: %w", ctx.Err())
	}
}
