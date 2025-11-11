package razorpay

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/domain/customer"
	"github.com/flexprice/flexprice/internal/domain/entityintegrationmapping"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/interfaces"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
)

// RazorpayCustomerService defines the interface for Razorpay customer operations
type RazorpayCustomerService interface {
	EnsureCustomerSyncedToRazorpay(ctx context.Context, customerID string, customerService interfaces.CustomerService) (*customer.Customer, error)
	SyncCustomerToRazorpay(ctx context.Context, flexpriceCustomer *customer.Customer) (string, error)
	GetRazorpayCustomerID(ctx context.Context, customerID string) (string, error)
}

// CustomerService handles Razorpay customer operations
type CustomerService struct {
	client                       RazorpayClient
	customerRepo                 customer.Repository
	entityIntegrationMappingRepo entityintegrationmapping.Repository
	logger                       *logger.Logger
}

// NewCustomerService creates a new Razorpay customer service
func NewCustomerService(
	client RazorpayClient,
	customerRepo customer.Repository,
	entityIntegrationMappingRepo entityintegrationmapping.Repository,
	logger *logger.Logger,
) RazorpayCustomerService {
	return &CustomerService{
		client:                       client,
		customerRepo:                 customerRepo,
		entityIntegrationMappingRepo: entityIntegrationMappingRepo,
		logger:                       logger,
	}
}

// EnsureCustomerSyncedToRazorpay ensures a customer is synced to Razorpay
func (s *CustomerService) EnsureCustomerSyncedToRazorpay(ctx context.Context, customerID string, customerService interfaces.CustomerService) (*customer.Customer, error) {
	s.logger.Infow("ensuring customer is synced to Razorpay",
		"customer_id", customerID)

	// Get FlexPrice customer
	customerResp, err := customerService.GetCustomer(ctx, customerID)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to get customer").
			WithReportableDetails(map[string]interface{}{
				"customer_id": customerID,
			}).
			Mark(ierr.ErrNotFound)
	}
	flexpriceCustomer := customerResp.Customer

	// Check if customer already has Razorpay mapping
	razorpayCustomerID, err := s.GetRazorpayCustomerID(ctx, customerID)
	if err == nil && razorpayCustomerID != "" {
		s.logger.Infow("customer already synced to Razorpay",
			"customer_id", customerID,
			"razorpay_customer_id", razorpayCustomerID)

		// Update customer metadata with Razorpay customer ID
		if flexpriceCustomer.Metadata == nil {
			flexpriceCustomer.Metadata = types.Metadata{}
		}
		flexpriceCustomer.Metadata["razorpay_customer_id"] = razorpayCustomerID

		return flexpriceCustomer, nil
	}

	// Customer not synced, create in Razorpay
	s.logger.Infow("customer not found in Razorpay, creating new customer",
		"customer_id", customerID)

	razorpayCustomerID, err = s.SyncCustomerToRazorpay(ctx, flexpriceCustomer)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to sync customer to Razorpay").
			WithReportableDetails(map[string]interface{}{
				"customer_id": customerID,
			}).
			Mark(ierr.ErrSystem)
	}

	// Update customer metadata
	if flexpriceCustomer.Metadata == nil {
		flexpriceCustomer.Metadata = types.Metadata{}
	}
	flexpriceCustomer.Metadata["razorpay_customer_id"] = razorpayCustomerID

	s.logger.Infow("successfully synced customer to Razorpay",
		"customer_id", customerID,
		"razorpay_customer_id", razorpayCustomerID)

	return flexpriceCustomer, nil
}

// SyncCustomerToRazorpay creates a customer in Razorpay and stores the mapping
func (s *CustomerService) SyncCustomerToRazorpay(ctx context.Context, flexpriceCustomer *customer.Customer) (string, error) {
	// Prepare customer data for Razorpay
	customerData := map[string]interface{}{
		"name": flexpriceCustomer.Name,
	}

	// Add email if available
	if flexpriceCustomer.Email != "" {
		customerData["email"] = flexpriceCustomer.Email
	}

	// Add contact/phone if available (using a generic contact field if available in metadata)
	// Note: FlexPrice customer model doesn't have a phone field, so we skip this for now
	// or could extract from metadata if needed

	// Add notes with FlexPrice customer ID
	customerData["notes"] = map[string]interface{}{
		"flexprice_customer_id": flexpriceCustomer.ID,
		"environment_id":        types.GetEnvironmentID(ctx),
	}

	s.logger.Infow("creating customer in Razorpay",
		"customer_id", flexpriceCustomer.ID)

	// Create customer in Razorpay using wrapper function
	razorpayCustomer, err := s.client.CreateCustomer(ctx, customerData)
	if err != nil {
		s.logger.Errorw("failed to create customer in Razorpay",
			"error", err,
			"customer_id", flexpriceCustomer.ID)
		return "", err
	}

	// Safely extract customer ID from response
	rawID, ok := razorpayCustomer["id"].(string)
	if !ok || rawID == "" {
		s.logger.Errorw("missing Razorpay customer id in response",
			"customer_id", flexpriceCustomer.ID)
		return "", ierr.NewError("razorpay customer id missing in response").
			WithHint("Check Razorpay CreateCustomer response payload").
			Mark(ierr.ErrSystem)
	}
	razorpayCustomerID := rawID

	s.logger.Infow("created customer in Razorpay",
		"customer_id", flexpriceCustomer.ID,
		"razorpay_customer_id", razorpayCustomerID)

	// Store mapping in entity_integration_mapping
	mapping := &entityintegrationmapping.EntityIntegrationMapping{
		ID:               types.GenerateUUIDWithPrefix(types.UUID_PREFIX_ENTITY_INTEGRATION_MAPPING),
		EntityID:         flexpriceCustomer.ID,
		EntityType:       types.IntegrationEntityTypeCustomer,
		ProviderType:     string(types.SecretProviderRazorpay),
		ProviderEntityID: razorpayCustomerID,
		Metadata: map[string]interface{}{
			"created_via":          "flexprice_to_provider",
			"razorpay_customer_id": razorpayCustomerID,
			"synced_at":            time.Now().UTC().Format(time.RFC3339),
		},
		EnvironmentID: types.GetEnvironmentID(ctx),
		BaseModel:     types.GetDefaultBaseModel(ctx),
	}

	err = s.entityIntegrationMappingRepo.Create(ctx, mapping)
	if err != nil {
		s.logger.Errorw("failed to store Razorpay customer mapping",
			"error", err,
			"customer_id", flexpriceCustomer.ID,
			"razorpay_customer_id", razorpayCustomerID)
		// Don't fail the entire operation if mapping storage fails
		// The customer was created successfully in Razorpay
	} else {
		s.logger.Infow("stored Razorpay customer mapping",
			"customer_id", flexpriceCustomer.ID,
			"razorpay_customer_id", razorpayCustomerID)
	}

	return razorpayCustomerID, nil
}

// GetRazorpayCustomerID retrieves the Razorpay customer ID for a FlexPrice customer
func (s *CustomerService) GetRazorpayCustomerID(ctx context.Context, customerID string) (string, error) {
	filter := &types.EntityIntegrationMappingFilter{
		EntityID:      customerID,
		EntityType:    types.IntegrationEntityTypeCustomer,
		ProviderTypes: []string{string(types.SecretProviderRazorpay)},
	}

	mappings, err := s.entityIntegrationMappingRepo.List(ctx, filter)
	if err != nil {
		return "", ierr.WithError(err).
			WithHint("Failed to get Razorpay customer mapping").
			Mark(ierr.ErrSystem)
	}

	if len(mappings) == 0 {
		return "", ierr.NewError("customer not found in Razorpay").
			WithHint("Customer has not been synced to Razorpay").
			Mark(ierr.ErrNotFound)
	}

	return mappings[0].ProviderEntityID, nil
}
