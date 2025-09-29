// internal/integrations/base/client/client.go
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// HTTPClient implements BaseClient interface
type HTTPClient struct {
	httpClient *http.Client
	baseURL    string
	auth       AuthConfig
	headers    map[string]string
}

// NewHTTPClient creates a new HTTP client
func NewHTTPClient(baseURL string, timeout time.Duration) *HTTPClient {
	return &HTTPClient{
		httpClient: &http.Client{
			Timeout: timeout,
		},
		baseURL: baseURL,
		headers: make(map[string]string),
	}
}

// Get performs GET request
func (c *HTTPClient) Get(ctx context.Context, path string, params map[string]string) (*http.Response, error) {
	fullURL := c.baseURL + path

	if len(params) > 0 {
		values := url.Values{}
		for k, v := range params {
			values.Add(k, v)
		}
		fullURL += "?" + values.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create GET request: %w", err)
	}

	c.setHeaders(req)
	c.setAuthentication(req)

	return c.httpClient.Do(req)
}

// Post performs POST request
func (c *HTTPClient) Post(ctx context.Context, path string, body interface{}) (*http.Response, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create POST request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	c.setHeaders(req)
	c.setAuthentication(req)

	return c.httpClient.Do(req)
}

// Put performs PUT request
func (c *HTTPClient) Put(ctx context.Context, path string, body interface{}) (*http.Response, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", c.baseURL+path, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create PUT request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	c.setHeaders(req)
	c.setAuthentication(req)

	return c.httpClient.Do(req)
}

// Delete performs DELETE request
func (c *HTTPClient) Delete(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "DELETE", c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create DELETE request: %w", err)
	}

	c.setHeaders(req)
	c.setAuthentication(req)

	return c.httpClient.Do(req)
}

// SetAuthentication sets authentication configuration
func (c *HTTPClient) SetAuthentication(auth AuthConfig) {
	c.auth = auth
}

// GetBaseURL returns the base URL
func (c *HTTPClient) GetBaseURL() string {
	return c.baseURL
}

// GetTimeout returns the timeout duration
func (c *HTTPClient) GetTimeout() time.Duration {
	return c.httpClient.Timeout
}

// IsHealthy checks if the client can make requests
func (c *HTTPClient) IsHealthy(ctx context.Context) error {
	resp, err := c.Get(ctx, "/", nil)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("service unavailable: status %d", resp.StatusCode)
	}

	return nil
}

// setHeaders sets common headers
func (c *HTTPClient) setHeaders(req *http.Request) {
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
}

// setAuthentication sets authentication headers
func (c *HTTPClient) setAuthentication(req *http.Request) {
	switch c.auth.Type {
	case AuthTypeAPIKey:
		req.Header.Set("Authorization", "Bearer "+c.auth.APIKey)
	case AuthTypeBasicAuth:
		req.SetBasicAuth(c.auth.APIKey, c.auth.Secret)
	case AuthTypeBearer:
		req.Header.Set("Authorization", "Bearer "+c.auth.Token)
	}
}
