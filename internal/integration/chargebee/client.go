package chargebee

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"

	"github.com/chargebee/chargebee-go/v3"
	"github.com/flexprice/flexprice/internal/domain/connection"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/security"
	"github.com/flexprice/flexprice/internal/types"
)

// ChargebeeClient defines the interface for Chargebee API operations
type ChargebeeClient interface {
	GetChargebeeConfig(ctx context.Context) (*ChargebeeConfig, error)
	GetDecryptedChargebeeConfig(conn *connection.Connection) (*ChargebeeConfig, error)
	HasChargebeeConnection(ctx context.Context) bool
	GetConnection(ctx context.Context) (*connection.Connection, error)
	InitializeChargebeeSDK(ctx context.Context) error
	VerifyWebhookSignature(ctx context.Context, payload []byte, signature string) error
}

// Client handles Chargebee API client setup and configuration
type Client struct {
	connectionRepo    connection.Repository
	encryptionService security.EncryptionService
	logger            *logger.Logger
	isInitialized     bool
}

// ChargebeeConfig holds decrypted Chargebee configuration
type ChargebeeConfig struct {
	Site          string // Chargebee site name (e.g., "acme-test")
	APIKey        string // Chargebee API key
	WebhookSecret string // Webhook secret for verification (optional)
}

// NewClient creates a new Chargebee client
func NewClient(
	connectionRepo connection.Repository,
	encryptionService security.EncryptionService,
	logger *logger.Logger,
) ChargebeeClient {
	return &Client{
		connectionRepo:    connectionRepo,
		encryptionService: encryptionService,
		logger:            logger,
		isInitialized:     false,
	}
}

// GetChargebeeConfig retrieves and decrypts Chargebee configuration for the current environment
func (c *Client) GetChargebeeConfig(ctx context.Context) (*ChargebeeConfig, error) {
	// Get Chargebee connection for this environment
	conn, err := c.connectionRepo.GetByProvider(ctx, types.SecretProviderChargebee)
	if err != nil {
		return nil, ierr.NewError("failed to get Chargebee connection").
			WithHint("Chargebee connection not configured for this environment").
			WithReportableDetails(map[string]interface{}{
				"error": err.Error(),
			}).
			Mark(ierr.ErrNotFound)
	}

	chargebeeConfig, err := c.GetDecryptedChargebeeConfig(conn)
	if err != nil {
		return nil, ierr.NewError("failed to get Chargebee configuration").
			WithHint("Invalid Chargebee configuration").
			Mark(ierr.ErrValidation)
	}

	// Validate required fields
	if chargebeeConfig.Site == "" {
		c.logger.Errorw("missing Chargebee site",
			"connection_id", conn.ID,
			"environment_id", conn.EnvironmentID)
		return nil, ierr.NewError("missing Chargebee site").
			WithHint("Configure Chargebee site in the connection settings").
			Mark(ierr.ErrValidation)
	}

	if chargebeeConfig.APIKey == "" {
		c.logger.Errorw("missing Chargebee API key",
			"connection_id", conn.ID,
			"environment_id", conn.EnvironmentID)
		return nil, ierr.NewError("missing Chargebee API key").
			WithHint("Configure Chargebee API key in the connection settings").
			Mark(ierr.ErrValidation)
	}

	return chargebeeConfig, nil
}

// GetDecryptedChargebeeConfig decrypts and returns Chargebee configuration
func (c *Client) GetDecryptedChargebeeConfig(conn *connection.Connection) (*ChargebeeConfig, error) {
	// Decrypt the connection metadata if it's encrypted
	decryptedMetadata, err := c.decryptConnectionMetadata(conn)
	if err != nil {
		return nil, err
	}

	// Extract Chargebee configuration from decrypted metadata
	chargebeeConfig := &ChargebeeConfig{}

	if site, exists := decryptedMetadata["site"]; exists {
		chargebeeConfig.Site = site
	}

	if apiKey, exists := decryptedMetadata["api_key"]; exists {
		chargebeeConfig.APIKey = apiKey
	}

	if webhookSecret, exists := decryptedMetadata["webhook_secret"]; exists {
		chargebeeConfig.WebhookSecret = webhookSecret
	}

	c.logger.Infow("retrieved Chargebee config",
		"site", chargebeeConfig.Site,
		"has_api_key", chargebeeConfig.APIKey != "",
		"has_webhook_secret", chargebeeConfig.WebhookSecret != "")

	return chargebeeConfig, nil
}

// decryptConnectionMetadata decrypts the connection encrypted secret data
func (c *Client) decryptConnectionMetadata(conn *connection.Connection) (types.Metadata, error) {
	// Check if the connection has encrypted secret data
	if conn.EncryptedSecretData.Chargebee == nil {
		c.logger.Warnw("no chargebee metadata found in encrypted secret data", "connection_id", conn.ID)
		return types.Metadata{}, nil
	}

	// For Chargebee connections, decrypt the structured metadata
	if conn.ProviderType == types.SecretProviderChargebee {
		if conn.EncryptedSecretData.Chargebee == nil {
			c.logger.Warnw("no chargebee metadata found", "connection_id", conn.ID)
			return types.Metadata{}, nil
		}

		// Site is not encrypted (it's public information)
		site := conn.EncryptedSecretData.Chargebee.Site

		// Decrypt API key
		apiKey, err := c.encryptionService.Decrypt(conn.EncryptedSecretData.Chargebee.APIKey)
		if err != nil {
			c.logger.Errorw("failed to decrypt API key", "connection_id", conn.ID, "error", err)
			return nil, ierr.NewError("failed to decrypt API key").Mark(ierr.ErrInternal)
		}

		// Decrypt webhook secret (optional field)
		var webhookSecret string
		if conn.EncryptedSecretData.Chargebee.WebhookSecret != "" {
			webhookSecret, err = c.encryptionService.Decrypt(conn.EncryptedSecretData.Chargebee.WebhookSecret)
			if err != nil {
				c.logger.Warnw("failed to decrypt webhook secret", "connection_id", conn.ID, "error", err)
				// Don't fail - webhook secret is optional
				webhookSecret = ""
			}
		}

		decryptedMetadata := types.Metadata{
			"site":           site,
			"api_key":        apiKey,
			"webhook_secret": webhookSecret,
		}

		c.logger.Infow("successfully decrypted chargebee credentials",
			"connection_id", conn.ID,
			"site", site,
			"has_api_key", apiKey != "",
			"has_webhook_secret", webhookSecret != "")

		return decryptedMetadata, nil
	}

	return types.Metadata{}, nil
}

// HasChargebeeConnection checks if Chargebee connection exists for the current environment
func (c *Client) HasChargebeeConnection(ctx context.Context) bool {
	conn, err := c.connectionRepo.GetByProvider(ctx, types.SecretProviderChargebee)
	return err == nil && conn != nil && conn.Status == types.StatusPublished
}

// GetConnection retrieves the Chargebee connection for the current context
func (c *Client) GetConnection(ctx context.Context) (*connection.Connection, error) {
	conn, err := c.connectionRepo.GetByProvider(ctx, types.SecretProviderChargebee)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to get Chargebee connection").
			Mark(ierr.ErrDatabase)
	}
	if conn == nil {
		return nil, ierr.NewError("Chargebee connection not found").
			WithHint("Chargebee connection not configured for this environment").
			Mark(ierr.ErrNotFound)
	}
	return conn, nil
}

// InitializeChargebeeSDK configures the global Chargebee SDK instance
// This should be called before making any Chargebee SDK API calls
func (c *Client) InitializeChargebeeSDK(ctx context.Context) error {
	if c.isInitialized {
		return nil
	}

	config, err := c.GetChargebeeConfig(ctx)
	if err != nil {
		return err
	}

	// Configure Chargebee SDK globally
	chargebee.Configure(config.APIKey, config.Site)

	c.isInitialized = true
	c.logger.Infow("initialized Chargebee SDK",
		"site", config.Site)

	return nil
}

// VerifyWebhookSignature verifies the Chargebee webhook signature
func (c *Client) VerifyWebhookSignature(ctx context.Context, payload []byte, signature string) error {
	config, err := c.GetChargebeeConfig(ctx)
	if err != nil {
		c.logger.Errorw("failed to get Chargebee config for signature verification", "error", err)
		return ierr.NewError("failed to verify webhook signature").
			WithHint("Unable to verify Chargebee webhook signature").
			Mark(ierr.ErrInternal)
	}

	// Check if webhook secret is configured
	if config.WebhookSecret == "" {
		c.logger.Warnw("webhook secret not configured, skipping signature verification")
		// Allow webhook to proceed without verification if secret is not configured
		// This is useful for initial testing or if webhooks are behind authentication
		return nil
	}

	// Verify signature using HMAC SHA256
	// Chargebee uses HMAC SHA256 to sign the webhook body
	mac := hmac.New(sha256.New, []byte(config.WebhookSecret))
	mac.Write(payload)
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	if expectedSignature != signature {
		c.logger.Errorw("webhook signature mismatch",
			"expected_signature_length", len(expectedSignature),
			"received_signature_length", len(signature),
			"payload_length", len(payload))
		return ierr.NewError("webhook signature verification failed").
			WithHint("Invalid webhook signature").
			Mark(ierr.ErrValidation)
	}

	c.logger.Infow("webhook signature verified successfully")
	return nil
}
