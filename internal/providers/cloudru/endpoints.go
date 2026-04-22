package cloudru

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

// EndpointsURI is the URI for getting the actual Cloud.ru API endpoints.
const EndpointsURI = "https://api.cloud.ru/endpoints"

// EndpointsResponse is a response from the Cloud.ru API.
type EndpointsResponse struct {
	// Endpoints contains the list of actual API addresses of Cloud.ru products.
	Endpoints []Endpoint `json:"endpoints"`
}

// Endpoint is a product API address.
type Endpoint struct {
	ID      string `json:"id"`
	Address string `json:"address"`
}

// getEndpoints returns the actual Cloud.ru API endpoints.
func getEndpoints(ctx context.Context, url string) (*EndpointsResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("construct HTTP request for cloud.ru endpoints: %w", err)
	}

	slog.InfoContext(ctx, "get endpoints from cloud.ru", slog.String("discovery_url", url))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get cloud.ru endpoints: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); err != nil {
			slog.Error("failed to close response body", slog.Any("err", cerr))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get cloud.ru endpoints: unexpected status code %d", resp.StatusCode)
	}

	var endpoints EndpointsResponse
	if err = json.NewDecoder(resp.Body).Decode(&endpoints); err != nil {
		return nil, fmt.Errorf("decode cloud.ru endpoints: %w", err)
	}

	return &endpoints, nil
}

// Get returns the API address of the product by its ID.
// If the product is not found, the function returns nil.
func (er *EndpointsResponse) Get(id string) *Endpoint {
	for i := range er.Endpoints {
		if er.Endpoints[i].ID == id {
			return &er.Endpoints[i]
		}
	}

	return nil
}
