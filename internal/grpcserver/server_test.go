package grpcserver

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/swarm-deploy/cloud-secrets/internal/api"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
	cloudsecretspb "github.com/swarm-deploy/cloud-secrets/pkg/api/cloudsecrets"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	grpc_health_v1 "google.golang.org/grpc/health/grpc_health_v1"
)

func TestNew_RegistersServices(t *testing.T) {
	t.Parallel()

	server := New("127.0.0.1:0", api.NewService("test", contracts.ProviderDefinition{}, nil))
	services := server.grpcServer.GetServiceInfo()

	assert.Contains(t, services, cloudsecretspb.Controller_ServiceDesc.ServiceName)
	assert.Contains(t, services, grpc_health_v1.Health_ServiceDesc.ServiceName)
}

func TestServer_Run_ReturnsErrorWhenAddressOccupied(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, listener.Close())
	})

	server := New(listener.Addr().String(), api.NewService("test", contracts.ProviderDefinition{}, nil))
	err = server.Run(context.Background())

	require.Error(t, err)
	assert.ErrorContains(t, err, "listen on gRPC address")
}

func TestServer_Health(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := New(listener.Addr().String(), api.NewService("test", contracts.ProviderDefinition{}, nil))
	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- server.serve(listener)
	}()

	connection, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, connection.Close())
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	healthClient := grpc_health_v1.NewHealthClient(connection)
	for _, serviceName := range []string{"", cloudsecretspb.Controller_ServiceDesc.ServiceName} {
		response, checkErr := healthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: serviceName})
		require.NoError(t, checkErr)
		assert.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, response.GetStatus())
	}

	require.NoError(t, server.Stop(ctx))
	require.NoError(t, <-serveErrors)
}
