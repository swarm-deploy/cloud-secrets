// Package certificateapi implements only the Certificate Manager reads used by cloud-secrets.
package certificateapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// DefaultAddress is the public HTTP endpoint documented by Cloud.ru.
const DefaultAddress = "https://certificatemanager.api.cloud.ru"

// Certificate contains the metadata required to synchronize one certificate.
type Certificate struct {
	// ID identifies the certificate within the project.
	ID string `json:"id"`
	// Description is the certificate's human-readable description.
	Description string `json:"description"`
}

// Version contains the small subset of version fields needed for synchronization.
type Version struct {
	// ID accepts both JSON numbers and protobuf JSON decimal strings.
	ID json.Number `json:"id"`
	// CertificateID identifies the owning certificate.
	CertificateID string `json:"certificateId"`
	// Enabled indicates whether this version may be used.
	Enabled bool `json:"enabled"`
	// Status indicates readiness of the certificate payload.
	Status string `json:"status"`
	// ExpiresAt is the API's expiration timestamp used to select usable versions.
	ExpiresAt string `json:"expiresAt"`
	// ServerCertificate is the leaf certificate in PEM form.
	ServerCertificate string `json:"serverCertificate"`
	// IntermediateCertificate is the intermediate certificate chain in PEM form.
	IntermediateCertificate string `json:"intermediateCertificate"`
}

// Material is the response for an explicitly selected version and its private key.
type Material struct {
	// PrivateKey is the private key in PEM form.
	PrivateKey string `json:"privateKey"`
	// Version describes the version to which PrivateKey belongs.
	Version Version `json:"version"`
}

// Client is a small handwritten HTTP client, sharing the provider's IAM token cache.
type Client struct {
	address string
	http    *http.Client
	token   func(context.Context) (string, error)
}

// NewClient constructs a client with mandatory HTTP transport and token acquisition.
func NewClient(address string, httpClient *http.Client, token func(context.Context) (string, error)) (*Client, error) {
	if !strings.Contains(address, "://") {
		address = "https://" + address
	}
	endpoint, err := url.Parse(address)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return nil, errors.New("invalid Certificate Manager HTTP address")
	}
	return &Client{address: strings.TrimRight(address, "/"), http: httpClient, token: token}, nil
}

// ListCertificates lists all active certificates; this API has no pagination.
func (c *Client) ListCertificates(ctx context.Context, projectID string) ([]Certificate, error) {
	var response struct {
		// Certificates contains all certificates matching the query.
		Certificates []Certificate `json:"certificates"`
	}
	query := url.Values{"projectId": {projectID}, "state": {"CERTIFICATE_STATE_ACTIVE"}}
	if err := c.get(ctx, "/v1/certificate", query, &response); err != nil {
		return nil, err
	}
	return response.Certificates, nil
}

// ListVersions reads every page of versions for a certificate.
func (c *Client) ListVersions(ctx context.Context, certificateID string) ([]Version, error) {
	var versions []Version
	seen := make(map[string]bool)
	pageToken := ""
	for {
		var response struct {
			// Versions contains the current page of certificate versions.
			Versions []Version `json:"versions"`
			// NextPageToken identifies the next page, or is empty for the final page.
			NextPageToken string `json:"nextPageToken"`
		}
		query := url.Values{"pageSize": {"100"}, "pageToken": {pageToken}}
		if err := c.get(ctx, "/v1/certificate/"+url.PathEscape(certificateID)+"/versions", query, &response); err != nil {
			return nil, err
		}
		versions = append(versions, response.Versions...)
		if response.NextPageToken == "" {
			return versions, nil
		}
		if seen[response.NextPageToken] {
			return nil, errors.New("Certificate Manager repeated a pagination token")
		}
		pageToken = response.NextPageToken
		seen[pageToken] = true
	}
}

// GetMaterial fetches the key and certificate for the exact selected version, never version zero/latest.
func (c *Client) GetMaterial(ctx context.Context, certificateID, versionID string) (Material, error) {
	var response Material
	path := "/v1/certificate/" + url.PathEscape(certificateID) + "/version/" + url.PathEscape(versionID) + ":private_key"
	err := c.get(ctx, path, nil, &response)
	return response, err
}

func (c *Client) get(ctx context.Context, path string, query url.Values, response any) error {
	token, err := c.token(ctx)
	if err != nil {
		return fmt.Errorf("get IAM token for Certificate Manager: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.address+path+"?"+query.Encode(), http.NoBody)
	if err != nil {
		return fmt.Errorf("construct Certificate Manager request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request Certificate Manager: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		// API error bodies may contain sensitive certificate material; do not include them.
		return fmt.Errorf("Certificate Manager returned HTTP %d", resp.StatusCode)
	}
	if err = json.NewDecoder(resp.Body).Decode(response); err != nil {
		return fmt.Errorf("decode Certificate Manager response: %w", err)
	}
	return nil
}
