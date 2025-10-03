// internal/integration/base/types.go
package base

import (
	"context"
)

// IntegrationType represents different payment gateway types
type IntegrationType string

const (
	IntegrationTypeStripe   IntegrationType = "stripe"
	IntegrationTypeRazorpay IntegrationType = "razorpay"
	IntegrationTypePayPal   IntegrationType = "paypal"
)

// Common statuses
const (
	StatusActive    = "active"
	StatusInactive  = "inactive"
	StatusPending   = "pending"
	StatusCancelled = "cancelled"
	StatusPaid      = "paid"
	StatusUnpaid    = "unpaid"
)

// GenericClient defines the interface for all payment gateway clients
type GenericClient interface {
	// Health check
	IsHealthy(ctx context.Context) error

	// Get the underlying client (could be HTTP client, SDK client, etc.)
	GetUnderlyingClient() interface{}

	// Get client type for debugging/logging
	GetClientType() string
}

// WebhookHandler defines the interface for webhook event processing
type WebhookHandler interface {
	// ProcessEvent processes a webhook event
	ProcessEvent(ctx context.Context, event WebhookEvent) (*WebhookResponse, error)

	// ValidateSignature validates the webhook signature
	ValidateSignature(payload []byte, signature string) error

	// GetSupportedEvents returns list of supported event types
	GetSupportedEvents() []string
}

// WebhookEvent represents a generic webhook event
type WebhookEvent struct {
	Type      string                 `json:"type"`
	ID        string                 `json:"id"`
	Created   int64                  `json:"created"`
	Data      map[string]interface{} `json:"data"`
	Raw       []byte                 `json:"-"`
	Signature string                 `json:"-"`
}

// WebhookResponse represents the response from webhook processing
type WebhookResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// IntegrationConfig represents configuration for an integration
type IntegrationConfig struct {
	Type          IntegrationType `json:"type"`
	APIKey        string          `json:"api_key"`
	Secret        string          `json:"secret"`
	WebhookSecret string          `json:"webhook_secret"`
	BaseURL       string          `json:"base_url,omitempty"`
	Timeout       int             `json:"timeout,omitempty"`
	APIVersion    string          `json:"api_version,omitempty"`
}

// Integration represents a complete payment gateway integration
type Integration struct {
	Type           IntegrationType
	Client         GenericClient
	WebhookHandler WebhookHandler
	Config         IntegrationConfig
}

// IntegrationManager manages all payment gateway integrations
type IntegrationManager interface {
	// RegisterIntegration registers a new integration
	RegisterIntegration(integration Integration) error

	// GetClient retrieves a client for a specific integration type
	GetClient(integrationType IntegrationType) (GenericClient, error)

	// GetWebhookHandler retrieves a webhook handler for a specific integration type
	GetWebhookHandler(integrationType IntegrationType) (WebhookHandler, error)

	// GetIntegration retrieves a complete integration
	GetIntegration(integrationType IntegrationType) (Integration, error)

	// ListIntegrations returns all registered integration types
	ListIntegrations() []IntegrationType

	// HealthCheck performs health check on all integrations
	HealthCheck(ctx context.Context) map[IntegrationType]error
}
