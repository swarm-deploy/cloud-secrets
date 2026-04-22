package cloudru

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	iamAuthV1 "github.com/cloudru-tech/iam-sdk/api/auth/v1"
	"google.golang.org/grpc/credentials"

	smssdk "github.com/cloudru-tech/secret-manager-sdk"
	"github.com/swarm-deploy/cloud-secrets/internal/metrics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

type Provider struct {
	cfg Config

	secretManager *smssdk.Client
	metrics       metrics.Provider
}

func NewProvider(ctx context.Context, cfg Config, providerMetrics metrics.Provider) (*Provider, error) {
	p := &Provider{
		cfg:     cfg,
		metrics: providerMetrics,
	}

	slog.Info("[cloudru] resolving endpoints")

	if err := p.resolveEndpoints(ctx); err != nil {
		return nil, fmt.Errorf("resolve endpoints: %w", err)
	}

	slog.Info("[cloudru] endpoints resolved")

	if err := p.initSecretManagerClient(); err != nil {
		return nil, fmt.Errorf("init secret manager client: %w", err)
	}

	return p, nil
}

func (p *Provider) resolveEndpoints(ctx context.Context) error {
	if p.cfg.IAM.Address != "" && p.cfg.CSM.Address != "" {
		return nil
	}

	discoveryURL := EndpointsURI
	if p.cfg.DiscoveryURL != "" {
		var u *url.URL
		u, err := url.Parse(p.cfg.DiscoveryURL)
		if err != nil {
			return fmt.Errorf("parse discovery URL: %w", err)
		}

		if u.Scheme != "https" && u.Scheme != "http" {
			return fmt.Errorf("invalid scheme in discovery URL, expected http or https, got %s", u.Scheme)
		}

		discoveryURL = p.cfg.DiscoveryURL
	}

	endpoints, err := getEndpoints(ctx, discoveryURL)
	if err != nil {
		return fmt.Errorf("get endpoints: %w", err)
	}

	smEndpoint := endpoints.Get("secret-manager")
	if smEndpoint == nil {
		return errors.New("secret-manager API is not available")
	}

	iamEndpoint := endpoints.Get("iam")
	if iamEndpoint == nil {
		return errors.New("iam API is not available")
	}

	p.cfg.CSM.Address = smEndpoint.Address
	p.cfg.IAM.Address = iamEndpoint.Address

	return nil
}

const (
	keepaliveTime    = time.Second * 30
	keepaliveTimeout = time.Second * 5
)

func (p *Provider) initSecretManagerClient() error {
	iamConn, err := grpc.NewClient(p.cfg.IAM.Address, grpc.WithTransportCredentials(
		credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13}),
	))
	if err != nil {
		return fmt.Errorf("create iam grpc client: %w", err)
	}

	iamClient := iamAuthV1.NewAuthServiceClient(iamConn)

	interceptor := iamInterceptor{
		iamClient:    iamClient,
		accessKey:    p.cfg.IAM.ClientID,
		accessSecret: p.cfg.IAM.ClientSecret,
	}

	smsClient, err := smssdk.New(&smssdk.Config{Host: p.cfg.CSM.Address},
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                keepaliveTime,
			Timeout:             keepaliveTimeout,
			PermitWithoutStream: false,
		}),
		grpc.WithUserAgent("docker-secret-volume"),
		grpc.WithUnaryInterceptor(interceptor.intercept()),
	)
	if err != nil {
		return err
	}

	p.secretManager = smsClient

	return nil
}
