// internal/integrations/base/client/interfaces.go
package client

import (
	"context"
	"net/http"
	"time"
)

// BaseClient defines the interface for all payment gateway clients
type BaseClient interface {
	// HTTP operations
	Get(ctx context.Context, path string, params map[string]string) (*http.Response, error)
	Post(ctx context.Context, path string, body interface{}) (*http.Response, error)
	Put(ctx context.Context, path string, body interface{}) (*http.Response, error)
	Delete(ctx context.Context, path string) (*http.Response, error)

	// Configuration
	GetBaseURL() string
	GetTimeout() time.Duration
	IsHealthy(ctx context.Context) error

	// Authentication
	SetAuthentication(auth AuthConfig)
}

// AuthConfig represents authentication configuration
type AuthConfig struct {
	Type   AuthType
	APIKey string
	Secret string
	Token  string
}

type AuthType string

const (
	AuthTypeAPIKey    AuthType = "api_key"
	AuthTypeBasicAuth AuthType = "basic_auth"
	AuthTypeBearer    AuthType = "bearer"
)
