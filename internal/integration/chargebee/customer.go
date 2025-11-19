package chargebee

import (
	"context"
	"time"

	"github.com/chargebee/chargebee-go/v3/enum"
	"github.com/chargebee/chargebee-go/v3/models/customer"
	"github.com/flexprice/flexprice/internal/api/dto"
	customerDomain "github.com/flexprice/flexprice/internal/domain/customer"
	"github.com/flexprice/flexprice/internal/domain/entityintegrationmapping"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/interfaces"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
)

// ChargebeeCustomerService defines the interface for Chargebee customer operations
type ChargebeeCustomerService interface {
	SyncCustomerToChargebee(ctx context.Context, flexpriceCustomer *customerDomain.Customer) (*CustomerResponse, error)
	GetOrCreateChargebeeCustomer(ctx context.Context, flexpriceCustomer *customerDomain.Customer) (string, error)
	GetChargebeeCustomerID(ctx context.Context, flexpriceCustomerID string) (string, error)
	EnsureCustomerSyncedToChargebee(ctx context.Context, customerID string, customerService interfaces.CustomerService) (*customerDomain.Customer, error)
}

// CustomerService handles Chargebee customer synchronization
type CustomerService struct {
	client                       ChargebeeClient
	customerRepo                 customerDomain.Repository
	entityIntegrationMappingRepo entityintegrationmapping.Repository
	logger                       *logger.Logger
}

// NewCustomerService creates a new Chargebee customer service
func NewCustomerService(
	client ChargebeeClient,
	customerRepo customerDomain.Repository,
	entityIntegrationMappingRepo entityintegrationmapping.Repository,
	logger *logger.Logger,
) ChargebeeCustomerService {
	return &CustomerService{
		client:                       client,
		customerRepo:                 customerRepo,
		entityIntegrationMappingRepo: entityIntegrationMappingRepo,
		logger:                       logger,
	}
}

// GetChargebeeCustomerID retrieves the Chargebee customer ID from entity mapping
func (s *CustomerService) GetChargebeeCustomerID(ctx context.Context, flexpriceCustomerID string) (string, error) {
	// Create filter for entity mapping lookup
	filter := types.NewEntityIntegrationMappingFilter()
	filter.EntityType = types.IntegrationEntityTypeCustomer
	filter.EntityID = flexpriceCustomerID
	filter.ProviderTypes = []string{string(types.SecretProviderChargebee)}

	// Get entity mapping
	mappings, err := s.entityIntegrationMappingRepo.List(ctx, filter)
	if err != nil {
		return "", ierr.NewError("failed to get entity mapping").
			WithReportableDetails(map[string]interface{}{
				"error":                 err.Error(),
				"flexprice_customer_id": flexpriceCustomerID,
			}).
			Mark(ierr.ErrNotFound)
	}

	if len(mappings) == 0 {
		return "", ierr.NewError("customer not synced to Chargebee").
			WithHint("Please sync customer to Chargebee first").
			WithReportableDetails(map[string]interface{}{
				"flexprice_customer_id": flexpriceCustomerID,
			}).
			Mark(ierr.ErrNotFound)
	}

	return mappings[0].ProviderEntityID, nil
}

// GetOrCreateChargebeeCustomer gets existing or creates a new customer in Chargebee
func (s *CustomerService) GetOrCreateChargebeeCustomer(ctx context.Context, flexpriceCustomer *customerDomain.Customer) (string, error) {
	// Check if customer already exists in Chargebee via entity mapping
	chargebeeCustomerID, err := s.GetChargebeeCustomerID(ctx, flexpriceCustomer.ID)
	if err == nil && chargebeeCustomerID != "" {
		s.logger.Infow("customer already synced to Chargebee",
			"flexprice_customer_id", flexpriceCustomer.ID,
			"chargebee_customer_id", chargebeeCustomerID)
		return chargebeeCustomerID, nil
	}

	// Customer doesn't exist in Chargebee, create new one
	s.logger.Infow("creating new customer in Chargebee",
		"flexprice_customer_id", flexpriceCustomer.ID,
		"email", flexpriceCustomer.Email)

	customerResp, err := s.SyncCustomerToChargebee(ctx, flexpriceCustomer)
	if err != nil {
		return "", err
	}

	return customerResp.ID, nil
}

// SyncCustomerToChargebee syncs FlexPrice customer to Chargebee
func (s *CustomerService) SyncCustomerToChargebee(ctx context.Context, flexpriceCustomer *customerDomain.Customer) (*CustomerResponse, error) {
	// Initialize Chargebee SDK
	if err := s.client.(*Client).InitializeChargebeeSDK(ctx); err != nil {
		return nil, err
	}

	s.logger.Infow("syncing customer to Chargebee",
		"customer_id", flexpriceCustomer.ID,
		"email", flexpriceCustomer.Email)

	// Prepare customer creation request
	createParams := &customer.CreateRequestParams{
		Email:          flexpriceCustomer.Email,
		AutoCollection: enum.AutoCollectionOn, // IMPORTANT: Set to "on" for Chargebee payments
	}

	// Add customer name
	if flexpriceCustomer.Name != "" {
		createParams.FirstName = flexpriceCustomer.Name
	}

	// Add billing address if available
	if flexpriceCustomer.AddressLine1 != "" {
		createParams.BillingAddress = &customer.CreateBillingAddressParams{
			Line1: flexpriceCustomer.AddressLine1,
		}

		if flexpriceCustomer.AddressLine2 != "" {
			createParams.BillingAddress.Line2 = flexpriceCustomer.AddressLine2
		}
		if flexpriceCustomer.AddressCity != "" {
			createParams.BillingAddress.City = flexpriceCustomer.AddressCity
		}
		if flexpriceCustomer.AddressState != "" {
			createParams.BillingAddress.State = flexpriceCustomer.AddressState
		}
		if flexpriceCustomer.AddressPostalCode != "" {
			createParams.BillingAddress.Zip = flexpriceCustomer.AddressPostalCode
		}
		if flexpriceCustomer.AddressCountry != "" {
			createParams.BillingAddress.Country = flexpriceCustomer.AddressCountry
		}
	}

	// Create customer in Chargebee using client wrapper
	result, err := s.client.CreateCustomer(ctx, createParams)
	if err != nil {
		s.logger.Errorw("failed to create customer in Chargebee",
			"customer_id", flexpriceCustomer.ID,
			"error", err)
		return nil, ierr.NewError("failed to create customer in Chargebee").
			WithReportableDetails(map[string]interface{}{
				"error":       err.Error(),
				"customer_id": flexpriceCustomer.ID,
			}).
			WithHint("Check Chargebee API credentials and customer data").
			Mark(ierr.ErrValidation)
	}

	chargebeeCustomer := result.Customer

	s.logger.Infow("successfully created customer in Chargebee",
		"flexprice_customer_id", flexpriceCustomer.ID,
		"chargebee_customer_id", chargebeeCustomer.Id,
		"email", chargebeeCustomer.Email)

	// Create entity mapping using repository Create method
	mapping := &entityintegrationmapping.EntityIntegrationMapping{
		ID:               types.GenerateUUIDWithPrefix(types.UUID_PREFIX_ENTITY_INTEGRATION_MAPPING),
		EntityType:       types.IntegrationEntityTypeCustomer,
		EntityID:         flexpriceCustomer.ID,
		ProviderType:     string(types.SecretProviderChargebee),
		ProviderEntityID: chargebeeCustomer.Id,
		EnvironmentID:    flexpriceCustomer.EnvironmentID,
		BaseModel:        types.GetDefaultBaseModel(ctx),
	}
	mapping.TenantID = flexpriceCustomer.TenantID

	err = s.entityIntegrationMappingRepo.Create(ctx, mapping)
	if err != nil {
		s.logger.Errorw("failed to save entity mapping",
			"customer_id", flexpriceCustomer.ID,
			"chargebee_customer_id", chargebeeCustomer.Id,
			"error", err)
		// Don't fail the entire operation, just log the error
	}

	// Convert to our DTO format
	customerResponse := &CustomerResponse{
		ID:              chargebeeCustomer.Id,
		FirstName:       chargebeeCustomer.FirstName,
		LastName:        chargebeeCustomer.LastName,
		Email:           chargebeeCustomer.Email,
		Company:         chargebeeCustomer.Company,
		Phone:           chargebeeCustomer.Phone,
		AutoCollection:  string(chargebeeCustomer.AutoCollection),
		CreatedAt:       time.Unix(chargebeeCustomer.CreatedAt, 0),
		UpdatedAt:       time.Unix(chargebeeCustomer.UpdatedAt, 0),
		ResourceVersion: chargebeeCustomer.ResourceVersion,
	}

	return customerResponse, nil
}

// EnsureCustomerSyncedToChargebee ensures a customer is synced to Chargebee
func (s *CustomerService) EnsureCustomerSyncedToChargebee(ctx context.Context, customerID string, customerService interfaces.CustomerService) (*customerDomain.Customer, error) {
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

	// Check if customer already has Chargebee ID in metadata
	if chargebeeID, exists := flexpriceCustomer.Metadata["chargebee_customer_id"]; exists && chargebeeID != "" {
		s.logger.Infow("customer already synced to Chargebee",
			"customer_id", customerID,
			"chargebee_customer_id", chargebeeID)
		return flexpriceCustomer, nil
	}

	// Check if customer is synced via integration mapping table
	if s.entityIntegrationMappingRepo != nil {
		filter := &types.EntityIntegrationMappingFilter{
			EntityID:      customerID,
			EntityType:    types.IntegrationEntityTypeCustomer,
			ProviderTypes: []string{string(types.SecretProviderChargebee)},
		}

		existingMappings, err := s.entityIntegrationMappingRepo.List(ctx, filter)
		if err == nil && existingMappings != nil && len(existingMappings) > 0 {
			existingMapping := existingMappings[0]
			s.logger.Infow("customer already mapped to Chargebee via integration mapping",
				"customer_id", customerID,
				"chargebee_customer_id", existingMapping.ProviderEntityID)

			// Update customer metadata with Chargebee ID for faster future lookups
			updateReq := dto.UpdateCustomerRequest{
				Metadata: s.mergeCustomerMetadata(flexpriceCustomer.Metadata, map[string]string{
					"chargebee_customer_id": existingMapping.ProviderEntityID,
				}),
			}
			updatedCustomerResp, err := customerService.UpdateCustomer(ctx, flexpriceCustomer.ID, updateReq)
			if err != nil {
				s.logger.Warnw("failed to update customer metadata with Chargebee customer ID",
					"customer_id", customerID,
					"error", err)
				// Return original customer info if update fails
				return flexpriceCustomer, nil
			}
			return updatedCustomerResp.Customer, nil
		}
	}

	// Customer is not synced, create in Chargebee
	s.logger.Infow("customer not synced to Chargebee, creating in Chargebee",
		"customer_id", customerID)
	err = s.CreateCustomerInChargebee(ctx, customerID, customerService)
	if err != nil {
		return nil, err
	}

	// Get updated customer after sync
	updatedCustomerResp, err := customerService.GetCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}

	return updatedCustomerResp.Customer, nil
}

// CreateCustomerInChargebee creates a customer in Chargebee and updates our customer with Chargebee ID
func (s *CustomerService) CreateCustomerInChargebee(ctx context.Context, customerID string, customerService interfaces.CustomerService) error {
	// Get FlexPrice customer
	customerResp, err := customerService.GetCustomer(ctx, customerID)
	if err != nil {
		return err
	}
	flexpriceCustomer := customerResp.Customer

	// Sync customer to Chargebee
	customerRespChargebee, err := s.SyncCustomerToChargebee(ctx, flexpriceCustomer)
	if err != nil {
		return err
	}

	chargebeeCustomerID := customerRespChargebee.ID

	// Update customer metadata with Chargebee ID
	updateReq := dto.UpdateCustomerRequest{
		Metadata: s.mergeCustomerMetadata(flexpriceCustomer.Metadata, map[string]string{
			"chargebee_customer_id": chargebeeCustomerID,
		}),
	}

	_, err = customerService.UpdateCustomer(ctx, customerID, updateReq)
	if err != nil {
		s.logger.Warnw("failed to update customer metadata with Chargebee customer ID",
			"customer_id", customerID,
			"chargebee_customer_id", chargebeeCustomerID,
			"error", err)
		// Don't fail the entire operation if metadata update fails
	}

	return nil
}

// mergeCustomerMetadata merges new metadata with existing customer metadata
func (s *CustomerService) mergeCustomerMetadata(existingMetadata map[string]string, newMetadata map[string]string) map[string]string {
	merged := make(map[string]string)

	// Copy existing metadata
	for k, v := range existingMetadata {
		merged[k] = v
	}

	// Add/override with new metadata
	for k, v := range newMetadata {
		merged[k] = v
	}

	return merged
}
