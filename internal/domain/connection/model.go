package connection

import (
	"github.com/flexprice/flexprice/ent"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
)

// Connection represents an integration connection in the system
type Connection struct {
	ID                  string                   `db:"id" json:"id"`
	Name                string                   `db:"name" json:"name"`
	ProviderType        types.SecretProvider     `db:"provider_type" json:"provider_type"`
	EncryptedSecretData types.ConnectionMetadata `db:"encrypted_secret_data" json:"encrypted_secret_data"`
	Metadata            map[string]interface{}   `db:"metadata" json:"metadata"`
	EnvironmentID       string                   `db:"environment_id" json:"environment_id"`
	types.BaseModel
}

// StripeConnection represents Stripe-specific connection metadata
type StripeConnection struct {
	PublishableKey string `json:"publishable_key"`
	SecretKey      string `json:"secret_key"`
	WebhookSecret  string `json:"webhook_secret"`
	AccountID      string `json:"account_id,omitempty"`
}

type SSLCommerzConnection struct {
	StoreID       string `json:"store_id"`
	StorePassword string `json:"store_password"`
}

// GetStripeConfig extracts Stripe configuration from connection metadata
func (c *Connection) GetStripeConfig() (*StripeConnection, error) {
	if c.ProviderType != types.SecretProviderStripe {
		return nil, ierr.NewError("connection is not a Stripe connection").
			WithHint("Connection provider type must be Stripe").
			Mark(ierr.ErrValidation)
	}

	if c.EncryptedSecretData.Stripe == nil {
		return nil, ierr.NewError("stripe metadata is not configured").
			WithHint("Stripe metadata is required for Stripe connections").
			Mark(ierr.ErrValidation)
	}

	config := &StripeConnection{
		PublishableKey: c.EncryptedSecretData.Stripe.PublishableKey,
		SecretKey:      c.EncryptedSecretData.Stripe.SecretKey,
		WebhookSecret:  c.EncryptedSecretData.Stripe.WebhookSecret,
		AccountID:      c.EncryptedSecretData.Stripe.AccountID,
	}

	return config, nil
}

func (c *Connection) GetSSLCommerzConfig() (*SSLCommerzConnection, error) {
	if c.ProviderType != types.SecretProviderSSLCommerz {
		return nil, ierr.NewError("connection is not an SSLCommerz connection").
			WithHint("Connection provider type must be SSLCommerz").
			Mark(ierr.ErrValidation)
	}

	if c.EncryptedSecretData.SSLCommerz == nil {
		return nil, ierr.NewError("sslcommerz metadata is not configured").
			WithHint("SSLCommerz metadata is required for SSLCommerz connections").
			Mark(ierr.ErrValidation)
	}

	config := &SSLCommerzConnection{
		StoreID:       c.EncryptedSecretData.SSLCommerz.StoreID,
		StorePassword: c.EncryptedSecretData.SSLCommerz.StorePassword,
	}

	return config, nil
}

// IsInvoiceSyncEnabled checks if invoice sync is enabled for this connection
func (c *Connection) IsInvoiceSyncEnabled() bool {
	if c.Metadata == nil {
		return false // Default to false if metadata is not set
	}

	// Check if invoice_sync_enable is set to true
	if enable, ok := c.Metadata["invoice_sync_enable"].(bool); ok {
		return enable
	}

	return false // Default to false if not set or not a boolean
}

// convertMapToConnectionMetadata converts old map format to new structured format
func convertMapToConnectionMetadata(metadata map[string]interface{}, providerType types.SecretProvider) types.ConnectionMetadata {
	switch providerType {
	case types.SecretProviderStripe:
		stripeMetadata := &types.StripeConnectionMetadata{}
		if pk, ok := metadata["publishable_key"].(string); ok {
			stripeMetadata.PublishableKey = pk
		}
		if sk, ok := metadata["secret_key"].(string); ok {
			stripeMetadata.SecretKey = sk
		}
		if ws, ok := metadata["webhook_secret"].(string); ok {
			stripeMetadata.WebhookSecret = ws
		}
		if aid, ok := metadata["account_id"].(string); ok {
			stripeMetadata.AccountID = aid
		}
		return types.ConnectionMetadata{
			Stripe: stripeMetadata,
		}
	case types.SecretProviderSSLCommerz:
		sslcommerzMetadata := &types.SSLCommerzConnectionMetadata{}
		if sid, ok := metadata["store_id"].(string); ok {
			sslcommerzMetadata.StoreID = sid
		}
		if spw, ok := metadata["store_password"].(string); ok {
			sslcommerzMetadata.StorePassword = spw
		}
		return types.ConnectionMetadata{
			SSLCommerz: sslcommerzMetadata,
		}
	default:
		// For other providers or unknown types, use generic format
		return types.ConnectionMetadata{
			Generic: &types.GenericConnectionMetadata{
				Data: metadata,
			},
		}
	}
}

// FromEnt converts an ent.Connection to domain Connection
func FromEnt(entConn *ent.Connection) *Connection {
	if entConn == nil {
		return nil
	}

	// Convert old map format to new structured format
	var metadata types.ConnectionMetadata
	if entConn.EncryptedSecretData != nil {
		metadata = convertMapToConnectionMetadata(entConn.EncryptedSecretData, types.SecretProvider(entConn.ProviderType))
	}

	return &Connection{
		ID:                  entConn.ID,
		Name:                entConn.Name,
		ProviderType:        types.SecretProvider(entConn.ProviderType),
		EncryptedSecretData: metadata,
		Metadata:            entConn.Metadata,
		EnvironmentID:       entConn.EnvironmentID,
		BaseModel: types.BaseModel{
			TenantID:  entConn.TenantID,
			Status:    types.Status(entConn.Status),
			CreatedAt: entConn.CreatedAt,
			UpdatedAt: entConn.UpdatedAt,
			CreatedBy: entConn.CreatedBy,
			UpdatedBy: entConn.UpdatedBy,
		},
	}
}
