package cloudru

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	iamAuthV1 "github.com/cloudru-tech/iam-sdk/api/auth/v1"
	"github.com/swarm-deploy/cloud-secrets/internal/grpcx"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/cloudru/certificateapi"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
	"google.golang.org/grpc/credentials"

	smssdk "github.com/cloudru-tech/secret-manager-sdk"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

type Provider struct {
	cfg Config

	secretManager       secretService
	certificateManager  *certificateapi.Client
	certificateMu       sync.RWMutex
	certificatePayloads map[string][]byte
}

func NewProvider(ctx context.Context, cfg Config) (*Provider, error) {
	p := &Provider{
		cfg: cfg,
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	slog.InfoContext(ctx, "[cloudru] resolving endpoints")

	if err := p.resolveEndpoints(ctx); err != nil {
		return nil, fmt.Errorf("resolve endpoints: %w", err)
	}

	slog.InfoContext(ctx, "[cloudru] endpoints resolved")

	if err := p.initSecretManagerClient(); err != nil {
		return nil, fmt.Errorf("init secret manager client: %w", err)
	}

	return p, nil
}

func (p *Provider) Definition() contracts.ProviderDefinition {
	return contracts.ProviderDefinition{
		Name: "Cloud.ru",
		Links: contracts.Links{
			Doc:     "https://cloud.ru/docs/scsm/ug/index",
			Manager: "https://console.cloud.ru/spa/secret-manager/list?projectId=" + url.QueryEscape(p.cfg.ProjectID),
		},
	}
}

func (p *Provider) resolveEndpoints(ctx context.Context) error {
	discoverCertificate := p.cfg.CertificateManager.Enabled && p.cfg.CertificateManager.Address == ""
	if discoverCertificate {
		p.cfg.CertificateManager.Address = certificateapi.DefaultAddress
	}
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
	if cmEndpoint := endpoints.Get("certificate-manager"); cmEndpoint != nil &&
		discoverCertificate {
		p.cfg.CertificateManager.Address = cmEndpoint.Address
	}

	return nil
}

const (
	keepaliveTime          = time.Second * 30
	keepaliveTimeout       = time.Second * 5
	certificateHTTPTimeout = 30 * time.Second
)

func (p *Provider) initSecretManagerClient() error {
	iamConn, err := grpc.NewClient(
		p.cfg.IAM.Address,
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13})),
		grpcx.WithUserAgent(),
	)
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
		grpcx.WithUserAgent(),
		grpc.WithUnaryInterceptor(interceptor.intercept()),
	)
	if err != nil {
		return err
	}

	p.secretManager = smsClient.V2.SecretService
	if p.cfg.CertificateManager.Enabled {
		cmClient, cmErr := certificateapi.NewClient(p.cfg.CertificateManager.Address,
			&http.Client{Timeout: certificateHTTPTimeout}, interceptor.getOrCreateToken)
		if cmErr != nil {
			return cmErr
		}
		p.certificateManager = cmClient
	}

	return nil
}
