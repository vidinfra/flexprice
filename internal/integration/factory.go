package integration

import (
	"context"

	"strings"

	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/domain/connection"
	"github.com/flexprice/flexprice/internal/domain/customer"
	"github.com/flexprice/flexprice/internal/domain/entityintegrationmapping"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/domain/payment"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/integration/stripe"
	"github.com/flexprice/flexprice/internal/integration/stripe/webhook"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/security"
	"github.com/flexprice/flexprice/internal/types"
)

// Factory manages different payment integration providers
type Factory struct {
	config                       *config.Configuration
	logger                       *logger.Logger
	connectionRepo               connection.Repository
	customerRepo                 customer.Repository
	invoiceRepo                  invoice.Repository
	paymentRepo                  payment.Repository
	entityIntegrationMappingRepo entityintegrationmapping.Repository
	encryptionService            security.EncryptionService
}

// NewFactory creates a new integration factory
func NewFactory(
	config *config.Configuration,
	logger *logger.Logger,
	connectionRepo connection.Repository,
	customerRepo customer.Repository,
	invoiceRepo invoice.Repository,
	paymentRepo payment.Repository,
	entityIntegrationMappingRepo entityintegrationmapping.Repository,
	encryptionService security.EncryptionService,
) *Factory {
	return &Factory{
		config:                       config,
		logger:                       logger,
		connectionRepo:               connectionRepo,
		customerRepo:                 customerRepo,
		invoiceRepo:                  invoiceRepo,
		paymentRepo:                  paymentRepo,
		entityIntegrationMappingRepo: entityIntegrationMappingRepo,
		encryptionService:            encryptionService,
	}
}

// GetStripeIntegration returns a complete Stripe integration setup
func (f *Factory) GetStripeIntegration(ctx context.Context) (*StripeIntegration, error) {
	// Create Stripe client
	stripeClient := stripe.NewClient(
		f.connectionRepo,
		f.encryptionService,
		f.logger,
	)

	// Create customer service
	customerSvc := stripe.NewCustomerService(
		stripeClient,
		f.customerRepo,
		f.entityIntegrationMappingRepo,
		f.logger,
	)

	// Create payment service
	paymentSvc := stripe.NewPaymentService(
		stripeClient,
		customerSvc,
		f.invoiceRepo,
		f.paymentRepo,
		f.logger,
	)

	// Create invoice sync service
	invoiceSyncSvc := stripe.NewInvoiceSyncService(
		stripeClient,
		customerSvc,
		f.invoiceRepo,
		f.entityIntegrationMappingRepo,
		f.logger,
	)

	// Create webhook handler
	webhookHandler := webhook.NewHandler(
		stripeClient,
		customerSvc,
		paymentSvc,
		invoiceSyncSvc,
		f.logger,
	)

	return &StripeIntegration{
		Client:         stripeClient,
		CustomerSvc:    customerSvc,
		PaymentSvc:     paymentSvc,
		InvoiceSyncSvc: invoiceSyncSvc,
		WebhookHandler: webhookHandler,
	}, nil
}

// GetIntegrationByProvider returns the appropriate integration for the given provider type
func (f *Factory) GetIntegrationByProvider(ctx context.Context, providerType types.SecretProvider) (interface{}, error) {
	switch providerType {
	case types.SecretProviderStripe:
		return f.GetStripeIntegration(ctx)
	default:
		return nil, ierr.NewError("unsupported integration provider").
			WithHint("Provider type is not supported").
			WithReportableDetails(map[string]interface{}{
				"provider_type": providerType,
			}).
			Mark(ierr.ErrValidation)
	}
}

// GetSupportedProviders returns all supported integration provider types
func (f *Factory) GetSupportedProviders() []types.SecretProvider {
	return []types.SecretProvider{
		types.SecretProviderStripe,
	}
}

// HasProvider checks if a provider is supported
func (f *Factory) HasProvider(providerType types.SecretProvider) bool {
	supportedProviders := f.GetSupportedProviders()
	for _, provider := range supportedProviders {
		if provider == providerType {
			return true
		}
	}
	return false
}

// StripeIntegration contains all Stripe integration services
type StripeIntegration struct {
	Client         *stripe.Client
	CustomerSvc    *stripe.CustomerService
	PaymentSvc     *stripe.PaymentService
	InvoiceSyncSvc *stripe.InvoiceSyncService
	WebhookHandler *webhook.Handler
}

// IntegrationProvider defines the interface for all integration providers
type IntegrationProvider interface {
	GetProviderType() types.SecretProvider
	IsAvailable(ctx context.Context) bool
}

// StripeProvider implements IntegrationProvider for Stripe
type StripeProvider struct {
	integration *StripeIntegration
}

// GetProviderType returns the provider type
func (p *StripeProvider) GetProviderType() types.SecretProvider {
	return types.SecretProviderStripe
}

// IsAvailable checks if Stripe integration is available
func (p *StripeProvider) IsAvailable(ctx context.Context) bool {
	return p.integration.Client.HasStripeConnection(ctx)
}

// GetAvailableProviders returns all available providers for the current environment
func (f *Factory) GetAvailableProviders(ctx context.Context) ([]IntegrationProvider, error) {
	var providers []IntegrationProvider

	// Check Stripe
	stripeIntegration, err := f.GetStripeIntegration(ctx)
	if err == nil {
		stripeProvider := &StripeProvider{integration: stripeIntegration}
		if stripeProvider.IsAvailable(ctx) {
			providers = append(providers, stripeProvider)
		}
	}

	// Future providers can be added here
	// razorpayIntegration, err := f.GetRazorpayIntegration(ctx)
	// if err == nil {
	//     razorpayProvider := &RazorpayProvider{integration: razorpayIntegration}
	//     if razorpayProvider.IsAvailable(ctx) {
	//         providers = append(providers, razorpayProvider)
	//     }
	// }

	return providers, nil
}

// ValidateProviderConnection validates a connection to a specific provider
func (f *Factory) ValidateProviderConnection(ctx context.Context, providerType types.SecretProvider, connectionData map[string]interface{}) error {
	switch providerType {
	case types.SecretProviderStripe:
		return f.validateStripeConnection(ctx, connectionData)
	default:
		return ierr.NewError("unsupported provider for validation").
			WithHint("Provider type is not supported for validation").
			WithReportableDetails(map[string]interface{}{
				"provider_type": providerType,
			}).
			Mark(ierr.ErrValidation)
	}
}

// validateStripeConnection validates Stripe connection data
func (f *Factory) validateStripeConnection(ctx context.Context, connectionData map[string]interface{}) error {
	// Extract required fields
	secretKey, ok := connectionData["secret_key"].(string)
	if !ok || secretKey == "" {
		return ierr.NewError("missing or invalid secret_key").
			WithHint("Stripe secret key is required").
			Mark(ierr.ErrValidation)
	}

	publishableKey, ok := connectionData["publishable_key"].(string)
	if !ok || publishableKey == "" {
		return ierr.NewError("missing or invalid publishable_key").
			WithHint("Stripe publishable key is required").
			Mark(ierr.ErrValidation)
	}

	// Test the connection by making a simple API call
	stripeClient := stripe.NewClient(
		f.connectionRepo,
		f.encryptionService,
		f.logger,
	)

	// This would typically make a test API call to validate the keys
	// For now, we'll just validate the format
	_ = stripeClient // Suppress unused variable warning
	if !strings.HasPrefix(secretKey, "sk_") {
		return ierr.NewError("invalid secret key format").
			WithHint("Stripe secret key should start with 'sk_'").
			Mark(ierr.ErrValidation)
	}

	if !strings.HasPrefix(publishableKey, "pk_") {
		return ierr.NewError("invalid publishable key format").
			WithHint("Stripe publishable key should start with 'pk_'").
			Mark(ierr.ErrValidation)
	}

	f.logger.Infow("Stripe connection validation successful",
		"secret_key_prefix", secretKey[:7]+"...",
		"publishable_key_prefix", publishableKey[:7]+"...")

	return nil
}
