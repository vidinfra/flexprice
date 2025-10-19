package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/connection"
	flexCustomer "github.com/flexprice/flexprice/internal/domain/customer"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/security"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"
)

// StripeService handles Stripe integration operations
type StripeService struct {
	ServiceParams
	encryptionService security.EncryptionService
}

// NewStripeService creates a new Stripe service instance
func NewStripeService(params ServiceParams) *StripeService {
	encryptionService, err := security.NewEncryptionService(params.Config, params.Logger)
	if err != nil {
		params.Logger.Fatalw("failed to create encryption service", "error", err)
	}

	return &StripeService{
		ServiceParams:     params,
		encryptionService: encryptionService,
	}
}

// mergeCustomerMetadata merges new metadata with existing customer metadata
func (s *StripeService) mergeCustomerMetadata(existingMetadata map[string]string, newMetadata map[string]string) map[string]string {
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

// decryptConnectionMetadata decrypts the connection encrypted secret data if it's encrypted
func (s *StripeService) decryptConnectionMetadata(encryptedSecretData types.ConnectionMetadata, providerType types.SecretProvider) (types.ConnectionMetadata, error) {
	decryptedMetadata := encryptedSecretData

	switch providerType {
	case types.SecretProviderStripe:
		if encryptedSecretData.Stripe != nil {
			decryptedPublishableKey, err := s.encryptionService.Decrypt(encryptedSecretData.Stripe.PublishableKey)
			if err != nil {
				return types.ConnectionMetadata{}, err
			}
			decryptedSecretKey, err := s.encryptionService.Decrypt(encryptedSecretData.Stripe.SecretKey)
			if err != nil {
				return types.ConnectionMetadata{}, err
			}
			decryptedWebhookSecret, err := s.encryptionService.Decrypt(encryptedSecretData.Stripe.WebhookSecret)
			if err != nil {
				return types.ConnectionMetadata{}, err
			}

			decryptedMetadata.Stripe = &types.StripeConnectionMetadata{
				PublishableKey: decryptedPublishableKey,
				SecretKey:      decryptedSecretKey,
				WebhookSecret:  decryptedWebhookSecret,
				AccountID:      encryptedSecretData.Stripe.AccountID, // Account ID is not sensitive
			}
		}

	default:
		// For other providers or unknown types, use generic format
		if encryptedSecretData.Generic != nil {
			decryptedData := make(map[string]interface{})
			for key, value := range encryptedSecretData.Generic.Data {
				if strValue, ok := value.(string); ok {
					decryptedValue, err := s.encryptionService.Decrypt(strValue)
					if err != nil {
						return types.ConnectionMetadata{}, err
					}
					decryptedData[key] = decryptedValue
				} else {
					decryptedData[key] = value
				}
			}
			decryptedMetadata.Generic = &types.GenericConnectionMetadata{
				Data: decryptedData,
			}
		}
	}

	return decryptedMetadata, nil
}

// GetDecryptedStripeConfig gets the decrypted Stripe configuration from a connection
func (s *StripeService) GetDecryptedStripeConfig(conn *connection.Connection) (*connection.StripeConnection, error) {
	// Decrypt metadata if needed
	decryptedMetadata, err := s.decryptConnectionMetadata(conn.EncryptedSecretData, conn.ProviderType)
	if err != nil {
		return nil, err
	}

	// Create a temporary connection with decrypted encrypted secret data
	tempConn := &connection.Connection{
		ID:                  conn.ID,
		Name:                conn.Name,
		ProviderType:        conn.ProviderType,
		EncryptedSecretData: decryptedMetadata,
		EnvironmentID:       conn.EnvironmentID,
		BaseModel:           conn.BaseModel,
	}

	// Now call GetStripeConfig on the decrypted connection
	return tempConn.GetStripeConfig()
}

// EnsureCustomerSyncedToStripe checks if customer is synced to Stripe and syncs if needed

func (s *StripeService) EnsureCustomerSyncedToStripe(ctx context.Context, customerID string) (*dto.CustomerResponse, error) {
	// Get our customer
	customerService := NewCustomerService(s.ServiceParams)
	ourCustomerResp, err := customerService.GetCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	ourCustomer := ourCustomerResp.Customer

	// Check if customer already has Stripe ID in metadata
	if stripeID, exists := ourCustomer.Metadata["stripe_customer_id"]; exists && stripeID != "" {
		s.Logger.Infow("customer already synced to Stripe",
			"customer_id", customerID,
			"stripe_customer_id", stripeID)
		return ourCustomerResp, nil
	}

	// Check if customer is synced via integration mapping table
	if s.EntityIntegrationMappingRepo != nil {
		filter := &types.EntityIntegrationMappingFilter{
			EntityID:      customerID,
			EntityType:    types.IntegrationEntityTypeCustomer,
			ProviderTypes: []string{string(types.SecretProviderStripe)},
		}

		entityMappingService := NewEntityIntegrationMappingService(s.ServiceParams)
		existingMappings, err := entityMappingService.GetEntityIntegrationMappings(ctx, filter)
		if err == nil && existingMappings != nil && len(existingMappings.Items) > 0 {
			existingMapping := existingMappings.Items[0]
			s.Logger.Infow("customer already mapped to Stripe via integration mapping",
				"customer_id", customerID,
				"stripe_customer_id", existingMapping.ProviderEntityID)

			// Update customer metadata with Stripe ID for faster future lookups
			updateReq := dto.UpdateCustomerRequest{
				Metadata: s.mergeCustomerMetadata(ourCustomer.Metadata, map[string]string{
					"stripe_customer_id": existingMapping.ProviderEntityID,
				}),
			}
			updatedCustomerResp, err := customerService.UpdateCustomer(ctx, ourCustomer.ID, updateReq)
			if err != nil {
				s.Logger.Warnw("failed to update customer metadata with Stripe ID",
					"customer_id", customerID,
					"error", err)
				// Return original customer info if update fails
				return ourCustomerResp, nil
			}
			return updatedCustomerResp, nil
		}
	}

	// Customer is not synced, create in Stripe
	s.Logger.Infow("customer not synced to Stripe, creating in Stripe",
		"customer_id", customerID)
	err = s.CreateCustomerInStripe(ctx, customerID)
	if err != nil {
		return nil, err
	}

	// Get updated customer after sync
	updatedCustomerResp, err := customerService.GetCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}

	return updatedCustomerResp, nil
}

// CreateCustomerInStripe creates a customer in Stripe and updates our customer with Stripe ID
func (s *StripeService) CreateCustomerInStripe(ctx context.Context, customerID string) error {
	// Get our customer
	customerService := NewCustomerService(s.ServiceParams)
	ourCustomerResp, err := customerService.GetCustomer(ctx, customerID)
	if err != nil {
		return err
	}
	ourCustomer := ourCustomerResp.Customer

	// Get Stripe connection for this environment
	conn, err := s.ConnectionRepo.GetByProvider(ctx, types.SecretProviderStripe)
	if err != nil {
		return ierr.NewError("failed to get Stripe connection").
			WithHint("Stripe connection not configured for this environment").
			Mark(ierr.ErrNotFound)
	}

	stripeConfig, err := s.GetDecryptedStripeConfig(conn)
	if err != nil {
		return ierr.NewError("failed to get Stripe configuration").
			WithHint("Invalid Stripe configuration").
			Mark(ierr.ErrValidation)
	}

	// Initialize Stripe client
	sc := stripe.NewClient(stripeConfig.SecretKey, nil)

	// Check if customer already has Stripe ID
	if stripeID, exists := ourCustomer.Metadata["stripe_customer_id"]; exists && stripeID != "" {
		return ierr.NewError("customer already has Stripe ID").
			WithHint("Customer is already synced with Stripe").
			Mark(ierr.ErrAlreadyExists)
	}

	// Create customer in Stripe
	params := &stripe.CustomerCreateParams{
		Name:  stripe.String(ourCustomer.Name),
		Email: stripe.String(ourCustomer.Email),
		Metadata: map[string]string{
			"flexprice_customer_id": ourCustomer.ID,
			"flexprice_environment": ourCustomer.EnvironmentID,
			"external_id":           ourCustomer.ExternalID,
		},
	}

	// Add address if available
	if ourCustomer.AddressLine1 != "" || ourCustomer.AddressCity != "" {
		params.Address = &stripe.AddressParams{
			Line1:      stripe.String(ourCustomer.AddressLine1),
			Line2:      stripe.String(ourCustomer.AddressLine2),
			City:       stripe.String(ourCustomer.AddressCity),
			State:      stripe.String(ourCustomer.AddressState),
			PostalCode: stripe.String(ourCustomer.AddressPostalCode),
			Country:    stripe.String(ourCustomer.AddressCountry),
		}
	}

	stripeCustomer, err := sc.V1Customers.Create(ctx, params)
	if err != nil {
		return ierr.NewError("failed to create customer in Stripe").
			WithHint("Stripe API error").
			Mark(ierr.ErrHTTPClient)
	}

	// Update our customer with Stripe ID
	updateReq := dto.UpdateCustomerRequest{
		Metadata: s.mergeCustomerMetadata(ourCustomer.Metadata, map[string]string{
			"stripe_customer_id": stripeCustomer.ID,
		}),
	}

	_, err = customerService.UpdateCustomer(ctx, ourCustomer.ID, updateReq)
	if err != nil {
		return err
	}

	// Create entity mapping
	entityMappingService := NewEntityIntegrationMappingService(s.ServiceParams)
	_, err = entityMappingService.CreateEntityIntegrationMapping(ctx, dto.CreateEntityIntegrationMappingRequest{
		EntityID:         ourCustomer.ID,
		EntityType:       types.IntegrationEntityTypeCustomer,
		ProviderType:     string(types.SecretProviderStripe),
		ProviderEntityID: stripeCustomer.ID,
		Metadata: map[string]interface{}{
			"created_via":           "flexprice_to_provider",
			"stripe_customer_email": ourCustomer.Email,
			"stripe_customer_name":  ourCustomer.Name,
			"synced_at":             time.Now().UTC().Format(time.RFC3339),
		},
	})
	if err != nil {
		s.Logger.Warnw("failed to create entity mapping for customer",
			"error", err,
			"customer_id", ourCustomer.ID,
			"stripe_customer_id", stripeCustomer.ID)
		// Don't fail the entire operation if entity mapping creation fails
	}

	return nil
}

// CreateCustomerFromStripe creates a customer in our system from Stripe webhook data
func (s *StripeService) CreateCustomerFromStripe(ctx context.Context, stripeCustomer *stripe.Customer, environmentID string) error {
	// Create customer service instance
	customerService := NewCustomerService(s.ServiceParams)

	// Check for existing customer by external ID if flexprice_customer_id is present
	var externalID string
	if flexpriceID, exists := stripeCustomer.Metadata["flexprice_customer_id"]; exists {
		externalID = flexpriceID
		// Check if customer with this external ID already exists
		existing, err := customerService.GetCustomerByLookupKey(ctx, externalID)
		if err == nil && existing != nil {
			// Customer exists with this external ID, update with Stripe ID
			updateReq := dto.UpdateCustomerRequest{
				Metadata: s.mergeCustomerMetadata(existing.Customer.Metadata, map[string]string{
					"stripe_customer_id": stripeCustomer.ID,
				}),
			}
			_, err = customerService.UpdateCustomer(ctx, existing.Customer.ID, updateReq)
			return err
		}
	} else {
		// When syncing from Stripe webhook, set external_id as stripe_customer_id
		externalID = stripeCustomer.ID
	}

	// Create new customer using DTO
	createReq := dto.CreateCustomerRequest{
		ExternalID: externalID,
		Name:       stripeCustomer.Name,
		Email:      stripeCustomer.Email,
		Metadata: map[string]string{
			"stripe_customer_id": stripeCustomer.ID,
		},
	}

	// Add address if available
	if stripeCustomer.Address != nil {
		createReq.AddressLine1 = stripeCustomer.Address.Line1
		createReq.AddressLine2 = stripeCustomer.Address.Line2
		createReq.AddressCity = stripeCustomer.Address.City
		createReq.AddressState = stripeCustomer.Address.State
		createReq.AddressPostalCode = stripeCustomer.Address.PostalCode
		createReq.AddressCountry = stripeCustomer.Address.Country
	}

	_, err := customerService.CreateCustomer(ctx, createReq)
	return err
}

// CreatePaymentLink creates a Stripe checkout session for payment
func (s *StripeService) CreatePaymentLink(ctx context.Context, req *dto.CreateStripePaymentLinkRequest) (*dto.StripePaymentLinkResponse, error) {
	s.Logger.Infow("creating stripe payment link",
		"invoice_id", req.InvoiceID,
		"customer_id", req.CustomerID,
		"amount", req.Amount.String(),
		"currency", req.Currency,
		"environment_id", req.EnvironmentID,
	)

	// Get Stripe connection for this environment
	conn, err := s.ConnectionRepo.GetByProvider(ctx, types.SecretProviderStripe)
	if err != nil {
		return nil, ierr.NewError("failed to get Stripe connection").
			WithHint("Stripe connection not configured for this environment").
			WithReportableDetails(map[string]interface{}{
				"environment_id": req.EnvironmentID,
			}).
			Mark(ierr.ErrNotFound)
	}

	// Validate invoice and check payment eligibility
	invoiceService := NewInvoiceService(s.ServiceParams)
	invoiceResp, err := invoiceService.GetInvoice(ctx, req.InvoiceID)
	if err != nil {
		return nil, ierr.NewError("failed to get invoice").
			WithHint("Invoice not found").
			WithReportableDetails(map[string]interface{}{
				"invoice_id": req.InvoiceID,
			}).
			Mark(ierr.ErrNotFound)
	}

	// Validate invoice payment status
	if invoiceResp.PaymentStatus == types.PaymentStatusSucceeded {
		return nil, ierr.NewError("invoice is already paid").
			WithHint("Cannot create payment link for an already paid invoice").
			WithReportableDetails(map[string]interface{}{
				"invoice_id":     req.InvoiceID,
				"payment_status": invoiceResp.PaymentStatus,
			}).
			Mark(ierr.ErrValidation)
	}

	if invoiceResp.InvoiceStatus == types.InvoiceStatusVoided {
		return nil, ierr.NewError("invoice is voided").
			WithHint("Cannot create payment link for a voided invoice").
			WithReportableDetails(map[string]interface{}{
				"invoice_id":     req.InvoiceID,
				"invoice_status": invoiceResp.InvoiceStatus,
			}).
			Mark(ierr.ErrValidation)
	}

	// Validate payment amount against invoice remaining balance
	if req.Amount.GreaterThan(invoiceResp.AmountRemaining) {
		return nil, ierr.NewError("payment amount exceeds invoice remaining balance").
			WithHint("Payment amount cannot be greater than the remaining balance on the invoice").
			WithReportableDetails(map[string]interface{}{
				"invoice_id":        req.InvoiceID,
				"payment_amount":    req.Amount.String(),
				"invoice_remaining": invoiceResp.AmountRemaining.String(),
				"invoice_total":     invoiceResp.AmountDue.String(),
				"invoice_paid":      invoiceResp.AmountPaid.String(),
			}).
			Mark(ierr.ErrValidation)
	}

	// Validate currency matches invoice currency
	if req.Currency != invoiceResp.Currency {
		return nil, ierr.NewError("payment currency does not match invoice currency").
			WithHint("Payment currency must match the invoice currency").
			WithReportableDetails(map[string]interface{}{
				"invoice_id":       req.InvoiceID,
				"payment_currency": req.Currency,
				"invoice_currency": invoiceResp.Currency,
			}).
			Mark(ierr.ErrValidation)
	}

	// Ensure customer is synced to Stripe before creating payment link
	customerResp, err := s.EnsureCustomerSyncedToStripe(ctx, req.CustomerID)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to sync customer to Stripe").
			WithReportableDetails(map[string]interface{}{
				"customer_id": req.CustomerID,
			}).
			Mark(ierr.ErrValidation)
	}

	// Get Stripe customer ID (should exist after sync)
	stripeCustomerID, exists := customerResp.Customer.Metadata["stripe_customer_id"]
	if !exists || stripeCustomerID == "" {
		return nil, ierr.NewError("customer does not have Stripe customer ID after sync").
			WithHint("Failed to sync customer to Stripe").
			WithReportableDetails(map[string]interface{}{
				"customer_id": req.CustomerID,
			}).
			Mark(ierr.ErrValidation)
	}

	// Get Stripe configuration
	stripeConfig, err := s.GetDecryptedStripeConfig(conn)
	if err != nil {
		return nil, ierr.NewError("failed to get Stripe configuration").
			WithHint("Invalid Stripe configuration").
			Mark(ierr.ErrValidation)
	}

	// Initialize Stripe client
	stripeClient := stripe.NewClient(stripeConfig.SecretKey, nil)

	// Convert amount to cents (Stripe expects amounts in smallest currency unit)
	amountCents := req.Amount.Mul(decimal.NewFromInt(100)).IntPart()

	// Build comprehensive product name with all information
	productName := fmt.Sprintf(customerResp.Customer.Name)

	// Build detailed description with all invoice information
	var descriptionParts []string

	// Add invoice information
	invoiceInfo := fmt.Sprintf("Invoice: %s", lo.FromPtrOr(invoiceResp.InvoiceNumber, req.InvoiceID))
	descriptionParts = append(descriptionParts, invoiceInfo)

	// Add invoice total
	totalInfo := fmt.Sprintf("Invoice Total: %s %s", invoiceResp.Total.String(), invoiceResp.Currency)
	descriptionParts = append(descriptionParts, totalInfo)

	// Add items details
	if len(invoiceResp.LineItems) > 0 {
		var itemDetails []string
		for _, lineItem := range invoiceResp.LineItems {
			if lineItem.Amount.IsZero() {
				continue // Skip zero-amount items
			}

			var entityType string
			var itemName string

			// Determine entity type and name using enums
			if lineItem.EntityType != nil {
				switch *lineItem.EntityType {
				case string(types.InvoiceLineItemEntityTypePlan):
					entityType = "Plan"
					itemName = lo.FromPtrOr(lineItem.DisplayName, "")
					if itemName == "" {
						itemName = lo.FromPtrOr(lineItem.PlanDisplayName, "Plan")
					}
				case string(types.InvoiceLineItemEntityTypeAddon):
					entityType = "Add-on"
					itemName = lo.FromPtrOr(lineItem.DisplayName, "Add-on")
				default:
					entityType = "Item"
					itemName = lo.FromPtrOr(lineItem.DisplayName, "Service")
				}
			}
			// Format as "Entity: Name ($Amount)"
			itemDetail := fmt.Sprintf("%s: %s ($%s)", entityType, itemName, lineItem.Amount.String())
			itemDetails = append(itemDetails, itemDetail)
		}

		if len(itemDetails) > 0 {
			descriptionParts = append(descriptionParts, itemDetails...)
		}
	}

	// Join all parts with separators for better readability
	productDescription := strings.Join(descriptionParts, " • ")

	// Create a single line item for the exact payment amount requested
	lineItems := []*stripe.CheckoutSessionCreateLineItemParams{
		{
			PriceData: &stripe.CheckoutSessionCreateLineItemPriceDataParams{
				Currency: stripe.String(req.Currency),
				ProductData: &stripe.CheckoutSessionCreateLineItemPriceDataProductDataParams{
					Name:        stripe.String(productName),
					Description: stripe.String(productDescription),
				},
				UnitAmount: stripe.Int64(amountCents),
			},
			Quantity: stripe.Int64(1),
		},
	}

	// Build metadata for the session
	metadata := map[string]string{
		"invoice_id":     req.InvoiceID,
		"customer_id":    req.CustomerID,
		"environment_id": req.EnvironmentID,
		"payment_source": "flexprice",
		"payment_type":   "checkout_link",
	}

	// Try to get Stripe invoice ID for attachment tracking
	if stripeInvoiceID, err := s.getStripeInvoiceID(ctx, req.InvoiceID); err == nil && stripeInvoiceID != "" {
		metadata["stripe_invoice_id"] = stripeInvoiceID
		s.Logger.Infow("payment link will be tracked for Stripe invoice attachment",
			"flexprice_invoice_id", req.InvoiceID,
			"stripe_invoice_id", stripeInvoiceID)
	} else {
		s.Logger.Debugw("no Stripe invoice found for payment link",
			"flexprice_invoice_id", req.InvoiceID,
			"error", err)
	}

	// Add custom metadata if provided
	for k, v := range req.Metadata {
		metadata[k] = v
	}

	// Provide default URLs if not provided
	successURL := req.SuccessURL
	if successURL == "" {
		successURL = "https://admin-dev.flexprice.io/customer-management/invoices?page=1"
	}

	cancelURL := req.CancelURL
	if cancelURL == "" {
		cancelURL = "https://admin-dev.flexprice.io/customer-management/invoices?page=1"
	}

	// Create checkout session parameters
	params := &stripe.CheckoutSessionCreateParams{
		LineItems:           lineItems,
		Mode:                stripe.String("payment"),
		AllowPromotionCodes: stripe.Bool(true),
		SuccessURL:          stripe.String(successURL),
		CancelURL:           stripe.String(cancelURL),
		Metadata:            metadata,
		Customer:            stripe.String(stripeCustomerID),
		PaymentIntentData: &stripe.CheckoutSessionCreatePaymentIntentDataParams{
			Metadata: metadata,
		},
	}

	// Only save payment method for future use if SaveCardAndMakeDefault is true
	if req.SaveCardAndMakeDefault {
		params.PaymentIntentData = &stripe.CheckoutSessionCreatePaymentIntentDataParams{
			SetupFutureUsage: stripe.String("off_session"),
			Metadata:         metadata,
		}
		s.Logger.Infow("payment link configured to save card and make default",
			"invoice_id", req.InvoiceID,
			"customer_id", req.CustomerID,
		)
	} else {
		s.Logger.Infow("payment link configured for one-time payment only",
			"invoice_id", req.InvoiceID,
			"customer_id", req.CustomerID,
		)
	}

	// Create the checkout session
	session, err := stripeClient.V1CheckoutSessions.Create(ctx, params)
	if err != nil {
		s.Logger.Errorw("failed to create Stripe checkout session",
			"error", err,
			"invoice_id", req.InvoiceID)
		return nil, ierr.NewError("failed to create payment link").
			WithHint("Unable to create Stripe checkout session").
			WithReportableDetails(map[string]interface{}{
				"invoice_id": req.InvoiceID,
				"error":      err.Error(),
			}).
			Mark(ierr.ErrSystem)
	}

	response := &dto.StripePaymentLinkResponse{
		ID:         session.ID,
		PaymentURL: session.URL,
		PaymentIntentID: func() string {
			if session.PaymentIntent != nil {
				return session.PaymentIntent.ID
			}
			return ""
		}(),
		Amount:    req.Amount,
		Currency:  req.Currency,
		Status:    string(session.Status),
		CreatedAt: session.Created,
		PaymentID: "", // Payment ID will be set by the calling code
	}

	s.Logger.Infow("successfully created stripe payment link",
		"payment_id", response.PaymentID,
		"session_id", session.ID,
		"payment_url", session.URL,
		"invoice_id", req.InvoiceID,
		"amount", req.Amount.String(),
		"currency", req.Currency,
	)

	return response, nil
}

// GetCustomerPaymentMethods retrieves saved payment methods for a customer
func (s *StripeService) GetCustomerPaymentMethods(ctx context.Context, req *dto.GetCustomerPaymentMethodsRequest) ([]*dto.PaymentMethodResponse, error) {
	// Get Stripe connection
	conn, err := s.ConnectionRepo.GetByProvider(ctx, types.SecretProviderStripe)
	if err != nil {
		return nil, ierr.NewError("failed to get Stripe connection").
			WithHint("Stripe connection not configured for this environment").
			Mark(ierr.ErrNotFound)
	}

	stripeConfig, err := s.GetDecryptedStripeConfig(conn)
	if err != nil {
		return nil, ierr.NewError("failed to get Stripe configuration").
			WithHint("Invalid Stripe configuration").
			Mark(ierr.ErrValidation)
	}

	// Initialize Stripe client
	stripeClient := stripe.NewClient(stripeConfig.SecretKey, nil)

	// Get our customer to find Stripe customer ID
	customerService := NewCustomerService(s.ServiceParams)
	ourCustomerResp, err := customerService.GetCustomer(ctx, req.CustomerID)
	if err != nil {
		return nil, err
	}
	ourCustomer := ourCustomerResp.Customer

	stripeCustomerID, exists := ourCustomer.Metadata["stripe_customer_id"]
	if !exists || stripeCustomerID == "" {
		// No Stripe customer ID means no saved payment methods
		s.Logger.Warnw("customer has no stripe_customer_id in metadata",
			"customer_id", req.CustomerID,
			"customer_metadata", ourCustomer.Metadata,
		)
		return []*dto.PaymentMethodResponse{}, nil
	}

	s.Logger.Infow("retrieving payment methods for stripe customer",
		"customer_id", req.CustomerID,
		"stripe_customer_id", stripeCustomerID,
	)

	// List payment methods for the customer
	params := &stripe.PaymentMethodListParams{
		Customer: stripe.String(stripeCustomerID),
		Type:     stripe.String("card"),
	}

	// get single customer
	customer, err := stripeClient.V1Customers.Retrieve(ctx, *params.Customer, nil)
	if err != nil {
		s.Logger.Errorw("failed to retrieve stripe customer",
			"error", err,
			"customer_id", req.CustomerID,
			"stripe_customer_id", stripeCustomerID)
		return nil, ierr.NewError("failed to retrieve stripe customer").
			WithHint("Unable to retrieve Stripe customer").
			WithReportableDetails(map[string]interface{}{
				"customer_id": req.CustomerID,
				"error":       err.Error(),
			}).
			Mark(ierr.ErrSystem)
	}

	paymentMethods := stripeClient.V1PaymentMethods.List(ctx, params)
	var responses []*dto.PaymentMethodResponse

	paymentMethods(func(pm *stripe.PaymentMethod, err error) bool {
		if err != nil {
			s.Logger.Errorw("failed to list payment methods",
				"error", err,
				"customer_id", req.CustomerID,
				"stripe_customer_id", stripeCustomerID)
			return false // Stop iteration on error
		}

		response := &dto.PaymentMethodResponse{
			ID:       pm.ID,
			Type:     string(pm.Type),
			Customer: pm.Customer.ID,
			Created:  pm.Created,
			Metadata: make(map[string]interface{}),
		}

		// Convert metadata from map[string]string to map[string]interface{}
		for k, v := range pm.Metadata {
			response.Metadata[k] = v
		}

		if pm.Card != nil {
			response.Card = &dto.CardDetails{
				Brand:       string(pm.Card.Brand),
				Last4:       pm.Card.Last4,
				ExpMonth:    int(pm.Card.ExpMonth),
				ExpYear:     int(pm.Card.ExpYear),
				Fingerprint: pm.Card.Fingerprint,
			}

			if customer.InvoiceSettings != nil && customer.InvoiceSettings.DefaultPaymentMethod != nil {
				response.IsDefault = customer.InvoiceSettings.DefaultPaymentMethod.ID == pm.ID
			}
		}

		responses = append(responses, response)
		return true // Continue iteration
	})

	if len(responses) == 0 {
		s.Logger.Warnw("no payment methods found for customer",
			"customer_id", req.CustomerID,
			"stripe_customer_id", stripeCustomerID)
		return responses, nil // Return empty list instead of error
	}

	s.Logger.Infow("successfully retrieved payment methods",
		"customer_id", req.CustomerID,
		"stripe_customer_id", stripeCustomerID,
		"payment_methods_count", len(responses),
	)

	return responses, nil
}

// SetDefaultPaymentMethod sets a payment method as default in Stripe
func (s *StripeService) SetDefaultPaymentMethod(ctx context.Context, customerID, paymentMethodID string) error {
	// Get Stripe connection
	conn, err := s.ConnectionRepo.GetByProvider(ctx, types.SecretProviderStripe)
	if err != nil {
		return ierr.NewError("failed to get Stripe connection").
			WithHint("Stripe connection not configured for this environment").
			Mark(ierr.ErrNotFound)
	}

	stripeConfig, err := s.GetDecryptedStripeConfig(conn)
	if err != nil {
		return ierr.NewError("failed to get Stripe configuration").
			WithHint("Invalid Stripe configuration").
			Mark(ierr.ErrValidation)
	}

	// Initialize Stripe client
	stripeClient := stripe.NewClient(stripeConfig.SecretKey, nil)

	// Get our customer to find Stripe customer ID
	customerService := NewCustomerService(s.ServiceParams)
	ourCustomerResp, err := customerService.GetCustomer(ctx, customerID)
	if err != nil {
		return err
	}
	ourCustomer := ourCustomerResp.Customer

	stripeCustomerID, exists := ourCustomer.Metadata["stripe_customer_id"]
	if !exists || stripeCustomerID == "" {
		return ierr.NewError("customer not found in Stripe").
			WithHint("Customer must have a Stripe account").
			Mark(ierr.ErrNotFound)
	}

	s.Logger.Infow("setting default payment method in Stripe",
		"customer_id", customerID,
		"stripe_customer_id", stripeCustomerID,
		"payment_method_id", paymentMethodID,
	)

	// Update customer's default payment method in Stripe
	params := &stripe.CustomerUpdateParams{
		InvoiceSettings: &stripe.CustomerUpdateInvoiceSettingsParams{
			DefaultPaymentMethod: stripe.String(paymentMethodID),
		},
	}

	_, err = stripeClient.V1Customers.Update(ctx, stripeCustomerID, params)
	if err != nil {
		s.Logger.Errorw("failed to set default payment method in Stripe",
			"error", err,
			"customer_id", customerID,
			"stripe_customer_id", stripeCustomerID,
			"payment_method_id", paymentMethodID,
		)
		return ierr.NewError("failed to set default payment method").
			WithHint("Could not update default payment method in Stripe").
			WithReportableDetails(map[string]interface{}{
				"customer_id":       customerID,
				"payment_method_id": paymentMethodID,
			}).
			Mark(ierr.ErrSystem)
	}

	s.Logger.Infow("successfully set default payment method in Stripe",
		"customer_id", customerID,
		"stripe_customer_id", stripeCustomerID,
		"payment_method_id", paymentMethodID,
	)

	return nil
}

// DetachPaymentMethod detaches a payment method from Stripe
func (s *StripeService) DetachPaymentMethod(ctx context.Context, paymentMethodID string) error {
	// Get Stripe connection
	conn, err := s.ConnectionRepo.GetByProvider(ctx, types.SecretProviderStripe)
	if err != nil {
		return ierr.NewError("failed to get Stripe connection").
			WithHint("Stripe connection not configured for this environment").
			Mark(ierr.ErrNotFound)
	}

	stripeConfig, err := s.GetDecryptedStripeConfig(conn)
	if err != nil {
		return ierr.NewError("failed to get Stripe configuration").
			WithHint("Invalid Stripe configuration").
			Mark(ierr.ErrValidation)
	}

	// Initialize Stripe client
	stripeClient := stripe.NewClient(stripeConfig.SecretKey, nil)

	s.Logger.Infow("deleting payment method from Stripe",
		"payment_method_id", paymentMethodID,
	)

	// Detach the payment method from the customer
	_, err = stripeClient.V1PaymentMethods.Detach(ctx, paymentMethodID, nil)
	if err != nil {
		s.Logger.Errorw("failed to delete payment method from Stripe",
			"error", err,
			"payment_method_id", paymentMethodID,
		)
		return ierr.NewError("failed to delete payment method").
			WithHint("Could not delete payment method from Stripe").
			WithReportableDetails(map[string]interface{}{
				"payment_method_id": paymentMethodID,
			}).
			Mark(ierr.ErrSystem)
	}

	s.Logger.Infow("successfully deleted payment method from Stripe",
		"payment_method_id", paymentMethodID,
	)

	return nil
}

// GetDefaultPaymentMethod retrieves the default payment method from Stripe
func (s *StripeService) GetDefaultPaymentMethod(ctx context.Context, customerID string) (*dto.PaymentMethodResponse, error) {
	// Get Stripe connection
	conn, err := s.ConnectionRepo.GetByProvider(ctx, types.SecretProviderStripe)
	if err != nil {
		return nil, ierr.NewError("failed to get Stripe connection").
			WithHint("Stripe connection not configured for this environment").
			Mark(ierr.ErrNotFound)
	}

	stripeConfig, err := s.GetDecryptedStripeConfig(conn)
	if err != nil {
		return nil, ierr.NewError("failed to get Stripe configuration").
			WithHint("Invalid Stripe configuration").
			Mark(ierr.ErrValidation)
	}

	// Initialize Stripe client
	stripeClient := stripe.NewClient(stripeConfig.SecretKey, nil)

	// Get our customer to find Stripe customer ID
	customerService := NewCustomerService(s.ServiceParams)
	ourCustomerResp, err := customerService.GetCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	ourCustomer := ourCustomerResp.Customer

	stripeCustomerID, exists := ourCustomer.Metadata["stripe_customer_id"]
	if !exists || stripeCustomerID == "" {
		return nil, ierr.NewError("customer not found in Stripe").
			WithHint("Customer must have a Stripe account").
			Mark(ierr.ErrNotFound)
	}

	// Get customer from Stripe to find default payment method
	customer, err := stripeClient.V1Customers.Retrieve(ctx, stripeCustomerID, nil)
	if err != nil {
		s.Logger.Errorw("failed to get customer from Stripe",
			"error", err,
			"customer_id", customerID,
			"stripe_customer_id", stripeCustomerID,
		)
		return nil, ierr.NewError("failed to get customer from Stripe").
			WithHint("Could not retrieve customer information from Stripe").
			Mark(ierr.ErrSystem)
	}

	// Check if customer has a default payment method
	if customer.InvoiceSettings == nil || customer.InvoiceSettings.DefaultPaymentMethod == nil {
		return nil, ierr.NewError("no default payment method").
			WithHint("Customer does not have a default payment method set in Stripe").
			WithReportableDetails(map[string]interface{}{
				"customer_id": customerID,
			}).
			Mark(ierr.ErrNotFound)
	}

	defaultPaymentMethodID := customer.InvoiceSettings.DefaultPaymentMethod.ID

	// Get the payment method details
	paymentMethod, err := stripeClient.V1PaymentMethods.Retrieve(ctx, defaultPaymentMethodID, nil)
	if err != nil {
		s.Logger.Errorw("failed to get default payment method from Stripe",
			"error", err,
			"customer_id", customerID,
			"payment_method_id", defaultPaymentMethodID,
		)
		return nil, ierr.NewError("failed to get payment method").
			WithHint("Could not retrieve payment method details from Stripe").
			Mark(ierr.ErrSystem)
	}

	// Convert to our DTO format
	response := &dto.PaymentMethodResponse{
		ID:       paymentMethod.ID,
		Type:     string(paymentMethod.Type),
		Customer: paymentMethod.Customer.ID,
		Created:  paymentMethod.Created,
		Metadata: make(map[string]interface{}),
	}

	// Convert metadata
	for k, v := range paymentMethod.Metadata {
		response.Metadata[k] = v
	}

	// Add card details if it's a card
	if paymentMethod.Type == stripe.PaymentMethodTypeCard && paymentMethod.Card != nil {
		response.Card = &dto.CardDetails{
			Brand:       string(paymentMethod.Card.Brand),
			Last4:       paymentMethod.Card.Last4,
			ExpMonth:    int(paymentMethod.Card.ExpMonth),
			ExpYear:     int(paymentMethod.Card.ExpYear),
			Fingerprint: paymentMethod.Card.Fingerprint,
		}
	}

	s.Logger.Infow("successfully retrieved default payment method",
		"customer_id", customerID,
		"stripe_customer_id", stripeCustomerID,
		"payment_method_id", defaultPaymentMethodID,
	)

	return response, nil
}

// ChargeSavedPaymentMethod charges a customer using their saved payment method
func (s *StripeService) ChargeSavedPaymentMethod(ctx context.Context, req *dto.ChargeSavedPaymentMethodRequest) (*dto.PaymentIntentResponse, error) {
	// Get Stripe connection
	conn, err := s.ConnectionRepo.GetByProvider(ctx, types.SecretProviderStripe)
	if err != nil {
		return nil, ierr.NewError("failed to get Stripe connection").
			WithHint("Stripe connection not configured for this environment").
			Mark(ierr.ErrNotFound)
	}

	stripeConfig, err := s.GetDecryptedStripeConfig(conn)
	if err != nil {
		return nil, ierr.NewError("failed to get Stripe configuration").
			WithHint("Invalid Stripe configuration").
			Mark(ierr.ErrValidation)
	}

	// Initialize Stripe client
	stripeClient := stripe.NewClient(stripeConfig.SecretKey, nil)

	// Ensure customer is synced to Stripe before charging saved payment method
	ourCustomerResp, err := s.EnsureCustomerSyncedToStripe(ctx, req.CustomerID)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to sync customer to Stripe").
			WithReportableDetails(map[string]interface{}{
				"customer_id": req.CustomerID,
			}).
			Mark(ierr.ErrValidation)
	}
	ourCustomer := ourCustomerResp.Customer

	stripeCustomerID, exists := ourCustomer.Metadata["stripe_customer_id"]
	if !exists || stripeCustomerID == "" {
		return nil, ierr.NewError("customer not found in Stripe after sync").
			WithHint("Failed to sync customer to Stripe").
			WithReportableDetails(map[string]interface{}{
				"customer_id": req.CustomerID,
			}).
			Mark(ierr.ErrValidation)
	}

	// Get invoice to validate payment amount
	invoiceService := NewInvoiceService(s.ServiceParams)
	invoiceResp, err := invoiceService.GetInvoice(ctx, req.InvoiceID)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to get invoice for payment validation").
			WithReportableDetails(map[string]interface{}{
				"invoice_id": req.InvoiceID,
			}).
			Mark(ierr.ErrNotFound)
	}
	// Validate payment amount against invoice remaining balance
	if req.Amount.GreaterThan(invoiceResp.AmountRemaining) {
		return nil, ierr.NewError("payment amount exceeds invoice remaining balance").
			WithHint("Payment amount cannot be greater than the remaining balance on the invoice").
			WithReportableDetails(map[string]interface{}{
				"invoice_id":        invoiceResp.ID,
				"payment_amount":    req.Amount.String(),
				"invoice_remaining": invoiceResp.AmountRemaining.String(),
				"invoice_total":     invoiceResp.AmountDue.String(),
				"invoice_paid":      invoiceResp.AmountPaid.String(),
			}).
			Mark(ierr.ErrValidation)
	}

	// Create PaymentIntent with saved payment method
	amountInCents := req.Amount.Mul(decimal.NewFromInt(100)).IntPart()
	params := &stripe.PaymentIntentCreateParams{
		Amount:        stripe.Int64(amountInCents),
		Currency:      stripe.String(req.Currency),
		Customer:      stripe.String(stripeCustomerID),
		PaymentMethod: stripe.String(req.PaymentMethodID),
		OffSession:    stripe.Bool(true), // Important: indicates off-session payment
		Confirm:       stripe.Bool(true), // Confirm immediately
		Metadata: map[string]string{
			"flexprice_customer_id": req.CustomerID,
			"environment_id":        types.GetEnvironmentID(ctx),
			"invoice_id":            req.InvoiceID,
			"payment_source":        "flexprice", // KEY DIFFERENTIATOR
		},
	}

	// Try to get Stripe invoice ID for later attachment
	stripeInvoiceID, err := s.getStripeInvoiceID(ctx, req.InvoiceID)
	if err != nil {
		s.Logger.Debugw("no Stripe invoice found, creating standalone payment",
			"flexprice_invoice_id", req.InvoiceID,
			"error", err)
		stripeInvoiceID = "" // Clear any partial value
	} else {
		s.Logger.Infow("will attach payment to Stripe invoice after successful payment",
			"flexprice_invoice_id", req.InvoiceID,
			"stripe_invoice_id", stripeInvoiceID,
			"payment_method_id", req.PaymentMethodID)
		// Add to metadata for tracking
		params.Metadata["stripe_invoice_id"] = stripeInvoiceID
	}

	paymentIntent, err := stripeClient.V1PaymentIntents.Create(ctx, params)
	if err != nil {
		// Handle specific error cases
		if stripeErr, ok := err.(*stripe.Error); ok {
			switch stripeErr.Code {
			case stripe.ErrorCodeAuthenticationRequired:
				// Payment requires authentication - customer needs to return to complete
				return nil, ierr.NewError("payment requires authentication").
					WithHint("Customer must return to complete payment authentication").
					WithReportableDetails(map[string]interface{}{
						"customer_id":       req.CustomerID,
						"payment_method_id": req.PaymentMethodID,
						"stripe_error_code": stripeErr.Code,
						"payment_intent_id": stripeErr.PaymentIntent.ID,
					}).
					Mark(ierr.ErrInvalidOperation)
			case stripe.ErrorCodeCardDeclined:
				// Card was declined
				return nil, ierr.NewError("payment method declined").
					WithHint("The saved payment method was declined").
					WithReportableDetails(map[string]interface{}{
						"customer_id":       req.CustomerID,
						"payment_method_id": req.PaymentMethodID,
						"stripe_error_code": stripeErr.Code,
					}).
					Mark(ierr.ErrInvalidOperation)
			}
		}

		s.Logger.Errorw("failed to create PaymentIntent with saved payment method",
			"error", err,
			"customer_id", req.CustomerID,
			"payment_method_id", req.PaymentMethodID,
			"amount", req.Amount.String(),
		)
		return nil, ierr.NewError("failed to charge saved payment method").
			WithHint("Unable to process payment with saved payment method").
			WithReportableDetails(map[string]interface{}{
				"customer_id":       req.CustomerID,
				"payment_method_id": req.PaymentMethodID,
				"error":             err.Error(),
			}).
			Mark(ierr.ErrSystem)
	}

	// If payment succeeded and we have a Stripe invoice, attach the payment to the invoice
	if paymentIntent.Status == stripe.PaymentIntentStatusSucceeded && stripeInvoiceID != "" {
		if err := s.AttachPaymentToStripeInvoice(ctx, stripeClient, paymentIntent.ID, stripeInvoiceID); err != nil {
			s.Logger.Errorw("failed to attach payment to Stripe invoice",
				"error", err,
				"payment_intent_id", paymentIntent.ID,
				"stripe_invoice_id", stripeInvoiceID)
			// Don't fail the whole payment, just log the error
			// The payment was successful, attachment is a bonus feature
		}
	}

	response := &dto.PaymentIntentResponse{
		ID:            paymentIntent.ID,
		Status:        string(paymentIntent.Status),
		Amount:        req.Amount,
		Currency:      req.Currency,
		CustomerID:    stripeCustomerID,
		PaymentMethod: req.PaymentMethodID,
		CreatedAt:     paymentIntent.Created,
	}

	s.Logger.Infow("successfully charged saved payment method",
		"payment_intent_id", paymentIntent.ID,
		"customer_id", req.CustomerID,
		"payment_method_id", req.PaymentMethodID,
		"amount", req.Amount.String(),
		"status", paymentIntent.Status,
		"stripe_invoice_id", stripeInvoiceID,
		"attached_to_invoice", stripeInvoiceID != "",
	)

	return response, nil
}

// HasSavedPaymentMethods checks if a customer has any saved payment methods
func (s *StripeService) HasSavedPaymentMethods(ctx context.Context, customerID string) (bool, error) {
	req := &dto.GetCustomerPaymentMethodsRequest{
		CustomerID: customerID,
	}

	paymentMethods, err := s.GetCustomerPaymentMethods(ctx, req)
	if err != nil {
		return false, err
	}

	return len(paymentMethods) > 0, nil
}

// HasCustomerStripeMapping checks if the customer has a Stripe entity mapping
func (s *StripeService) HasCustomerStripeMapping(ctx context.Context, customerID string) bool {
	entityMappingService := NewEntityIntegrationMappingService(s.ServiceParams)

	filter := &types.EntityIntegrationMappingFilter{
		EntityID:      customerID,
		EntityType:    types.IntegrationEntityTypeCustomer,
		ProviderTypes: []string{string(types.SecretProviderStripe)},
	}

	mappings, err := entityMappingService.GetEntityIntegrationMappings(ctx, filter)
	if err != nil {
		s.Logger.Debugw("failed to check customer Stripe mapping",
			"customer_id", customerID,
			"error", err,
		)
		return false
	}

	if mappings == nil || len(mappings.Items) == 0 {
		s.Logger.Debugw("no Stripe mapping found for customer",
			"customer_id", customerID,
		)
		return false
	}

	// Check if any mapping has a valid provider entity ID
	for _, mapping := range mappings.Items {
		if mapping.ProviderEntityID != "" {
			s.Logger.Debugw("customer has Stripe mapping",
				"customer_id", customerID,
				"provider_entity_id", mapping.ProviderEntityID,
			)
			return true
		}
	}

	s.Logger.Debugw("customer Stripe mapping found but no provider entity ID",
		"customer_id", customerID,
	)
	return false
}

// ReconcilePaymentWithInvoice updates the invoice payment status and amounts when a payment succeeds
func (s *StripeService) ReconcilePaymentWithInvoice(ctx context.Context, paymentID string, paymentAmount decimal.Decimal) error {
	s.Logger.Infow("starting payment reconciliation with invoice",
		"payment_id", paymentID,
		"payment_amount", paymentAmount.String(),
	)

	// Get the payment record
	paymentService := NewPaymentService(s.ServiceParams)
	payment, err := paymentService.GetPayment(ctx, paymentID)
	if err != nil {
		s.Logger.Errorw("failed to get payment record for reconciliation",
			"error", err,
			"payment_id", paymentID,
		)
		return err
	}

	s.Logger.Infow("got payment record for reconciliation",
		"payment_id", paymentID,
		"invoice_id", payment.DestinationID,
		"payment_amount", payment.Amount.String(),
	)

	// Get the invoice
	invoiceService := NewInvoiceService(s.ServiceParams)
	invoiceResp, err := invoiceService.GetInvoice(ctx, payment.DestinationID)
	if err != nil {
		s.Logger.Errorw("failed to get invoice for payment reconciliation",
			"error", err,
			"payment_id", paymentID,
			"invoice_id", payment.DestinationID,
		)
		return ierr.WithError(err).
			WithHint("Failed to get invoice for payment reconciliation").
			WithReportableDetails(map[string]interface{}{
				"payment_id": paymentID,
				"invoice_id": payment.DestinationID,
			}).
			Mark(ierr.ErrSystem)
	}

	s.Logger.Infow("got invoice for reconciliation",
		"payment_id", paymentID,
		"invoice_id", payment.DestinationID,
		"invoice_amount_due", invoiceResp.AmountDue.String(),
		"invoice_amount_paid", invoiceResp.AmountPaid.String(),
		"invoice_amount_remaining", invoiceResp.AmountRemaining.String(),
		"invoice_payment_status", invoiceResp.PaymentStatus,
		"invoice_status", invoiceResp.InvoiceStatus,
	)

	// Calculate new amounts
	newAmountPaid := invoiceResp.AmountPaid.Add(paymentAmount)
	newAmountRemaining := invoiceResp.AmountDue.Sub(newAmountPaid)

	// Determine payment status
	var newPaymentStatus types.PaymentStatus
	if newAmountRemaining.IsZero() {
		newPaymentStatus = types.PaymentStatusSucceeded
	} else if newAmountRemaining.IsNegative() {
		// Invoice is overpaid
		newPaymentStatus = types.PaymentStatusOverpaid
		// For overpaid invoices, amount_remaining should be 0
		newAmountRemaining = decimal.Zero
	} else {
		newPaymentStatus = types.PaymentStatusPending
	}

	s.Logger.Infow("calculated new amounts for reconciliation",
		"payment_id", paymentID,
		"invoice_id", payment.DestinationID,
		"payment_amount", paymentAmount.String(),
		"new_amount_paid", newAmountPaid.String(),
		"new_amount_remaining", newAmountRemaining.String(),
		"new_payment_status", newPaymentStatus,
	)

	// Update invoice payment status and amounts using reconciliation method
	s.Logger.Infow("calling invoice reconciliation",
		"payment_id", paymentID,
		"invoice_id", payment.DestinationID,
		"payment_amount", paymentAmount.String(),
		"new_payment_status", newPaymentStatus,
	)

	err = invoiceService.ReconcilePaymentStatus(ctx, payment.DestinationID, newPaymentStatus, &paymentAmount)
	if err != nil {
		s.Logger.Errorw("failed to update invoice payment status during reconciliation",
			"error", err,
			"payment_id", paymentID,
			"invoice_id", payment.DestinationID,
			"payment_amount", paymentAmount.String(),
			"new_payment_status", newPaymentStatus,
		)
		return ierr.WithError(err).
			WithHint("Failed to update invoice payment status").
			WithReportableDetails(map[string]interface{}{
				"payment_id":         paymentID,
				"invoice_id":         payment.DestinationID,
				"payment_amount":     paymentAmount.String(),
				"new_payment_status": newPaymentStatus,
			}).
			Mark(ierr.ErrSystem)
	}

	s.Logger.Infow("successfully reconciled payment with invoice",
		"payment_id", paymentID,
		"invoice_id", payment.DestinationID,
		"payment_amount", paymentAmount.String(),
		"new_payment_status", newPaymentStatus,
		"new_amount_paid", newAmountPaid.String(),
		"new_amount_remaining", newAmountRemaining.String(),
	)

	return nil
}

// ParseWebhookEvent parses a Stripe webhook event with signature verification
func (s *StripeService) ParseWebhookEvent(payload []byte, signature string, webhookSecret string) (*stripe.Event, error) {
	// Verify the webhook signature, ignoring API version mismatch
	options := webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	}
	event, err := webhook.ConstructEventWithOptions(payload, signature, webhookSecret, options)
	if err != nil {
		// Log the error using structured logging
		s.Logger.Errorw("Stripe webhook verification failed", "error", err)
		return nil, ierr.NewError("failed to verify webhook signature").
			WithHint("Invalid webhook signature or payload").
			Mark(ierr.ErrValidation)
	}
	return &event, nil
}

// VerifyWebhookSignature verifies the Stripe webhook signature
func (s *StripeService) VerifyWebhookSignature(payload []byte, signature string, webhookSecret string) error {
	_, err := webhook.ConstructEvent(payload, signature, webhookSecret)
	if err != nil {
		return ierr.NewError("failed to verify webhook signature").
			WithHint("Invalid webhook signature or payload").
			Mark(ierr.ErrValidation)
	}
	return nil
}

// GetPaymentStatus gets the payment status from Stripe checkout session
func (s *StripeService) GetPaymentStatus(ctx context.Context, sessionID string, environmentID string) (*dto.PaymentStatusResponse, error) {
	// Get Stripe connection for this environment
	conn, err := s.ConnectionRepo.GetByProvider(ctx, types.SecretProviderStripe)
	if err != nil {
		return nil, ierr.NewError("failed to get Stripe connection").
			WithHint("Stripe connection not configured for this environment").
			WithReportableDetails(map[string]interface{}{
				"environment_id": environmentID,
			}).
			Mark(ierr.ErrNotFound)
	}

	// Get Stripe configuration
	stripeConfig, err := s.GetDecryptedStripeConfig(conn)
	if err != nil {
		return nil, ierr.NewError("failed to get Stripe configuration").
			WithHint("Invalid Stripe configuration").
			Mark(ierr.ErrValidation)
	}

	// Initialize Stripe client
	stripeClient := stripe.NewClient(stripeConfig.SecretKey, nil)

	// Get the checkout session with expanded fields
	params := &stripe.CheckoutSessionRetrieveParams{
		Expand: []*string{
			stripe.String("payment_intent"),
			stripe.String("line_items"),
			stripe.String("customer"),
		},
	}
	session, err := stripeClient.V1CheckoutSessions.Retrieve(ctx, sessionID, params)
	if err != nil {
		s.Logger.Errorw("failed to get Stripe checkout session",
			"error", err,
			"session_id", sessionID)
		return nil, ierr.NewError("failed to get payment status").
			WithHint("Unable to retrieve Stripe checkout session").
			WithReportableDetails(map[string]interface{}{
				"session_id": sessionID,
				"error":      err.Error(),
			}).
			Mark(ierr.ErrSystem)
	}

	// Log session details for debugging
	s.Logger.Debugw("retrieved Stripe checkout session",
		"session_id", session.ID,
		"status", session.Status,
		"has_payment_intent", session.PaymentIntent != nil,
		"has_line_items", session.LineItems != nil,
		"line_items_count", func() int {
			if session.LineItems != nil {
				return len(session.LineItems.Data)
			}
			return 0
		}(),
		"has_customer", session.Customer != nil,
	)

	// Get payment intent if available
	var paymentIntentID string
	var paymentStatus string
	var amount decimal.Decimal
	var currency string
	var paymentMethodID string

	// First try to get data from payment intent
	if session.PaymentIntent != nil {
		paymentIntentID = session.PaymentIntent.ID
		paymentStatus = string(session.PaymentIntent.Status)
		if session.PaymentIntent.Amount > 0 {
			amount = decimal.NewFromInt(session.PaymentIntent.Amount).Div(decimal.NewFromInt(100))
		}
		if session.PaymentIntent.Currency != "" {
			currency = string(session.PaymentIntent.Currency)
		}

		// Get payment method ID from payment intent
		if paymentIntentID != "" {
			paymentIntent, err := stripeClient.V1PaymentIntents.Retrieve(ctx, paymentIntentID, nil)
			if err != nil {
				s.Logger.Warnw("failed to get payment intent details",
					"error", err,
					"payment_intent_id", paymentIntentID)
				// Don't fail the entire request if we can't get payment intent details
			} else {
				// Get the payment method ID from the payment intent
				if paymentIntent.PaymentMethod != nil {
					paymentMethodID = paymentIntent.PaymentMethod.ID
				}
			}
		}
	}

	// If payment intent data is incomplete, try to get from session
	if paymentStatus == "" {
		paymentStatus = string(session.Status)
	}

	// If amount is still 0, try to get from line items
	if amount.IsZero() && session.LineItems != nil && len(session.LineItems.Data) > 0 {
		item := session.LineItems.Data[0]
		if item.AmountTotal > 0 {
			amount = decimal.NewFromInt(item.AmountTotal).Div(decimal.NewFromInt(100))
		}
		if item.Currency != "" && currency == "" {
			currency = string(item.Currency)
		}
	}

	// If currency is still empty, try to get from session metadata or default
	if currency == "" {
		// Check if currency is in metadata
		if session.Metadata != nil {
			if curr, exists := session.Metadata["currency"]; exists {
				currency = curr
			}
		}
		// Default to USD if still empty
		if currency == "" {
			currency = "usd"
		}
	}

	// Log extracted values for debugging
	s.Logger.Debugw("extracted payment status values",
		"session_id", session.ID,
		"payment_intent_id", paymentIntentID,
		"status", paymentStatus,
		"amount", amount.String(),
		"currency", currency,
		"customer_id", func() string {
			if session.Customer != nil {
				return session.Customer.ID
			}
			return ""
		}(),
	)

	return &dto.PaymentStatusResponse{
		SessionID:       session.ID,
		PaymentIntentID: paymentIntentID,
		PaymentMethodID: paymentMethodID,
		Status:          paymentStatus,
		Amount:          amount,
		Currency:        currency,
		CustomerID: func() string {
			if session.Customer != nil {
				return session.Customer.ID
			}
			return ""
		}(),
		CreatedAt: session.Created,
		ExpiresAt: session.ExpiresAt,
		Metadata:  session.Metadata,
	}, nil
}

// GetPaymentStatusByPaymentIntent gets payment status directly from a payment intent ID
func (s *StripeService) GetPaymentStatusByPaymentIntent(ctx context.Context, paymentIntentID string, environmentID string) (*dto.PaymentStatusResponse, error) {
	// Get Stripe connection for this environment
	conn, err := s.ConnectionRepo.GetByProvider(ctx, types.SecretProviderStripe)
	if err != nil {
		return nil, ierr.NewError("failed to get Stripe connection").
			WithHint("Stripe connection not configured for this environment").
			WithReportableDetails(map[string]interface{}{
				"environment_id": environmentID,
			}).
			Mark(ierr.ErrNotFound)
	}

	// Get Stripe configuration
	stripeConfig, err := s.GetDecryptedStripeConfig(conn)
	if err != nil {
		return nil, ierr.NewError("failed to get Stripe configuration").
			WithHint("Invalid Stripe configuration").
			Mark(ierr.ErrValidation)
	}

	// Initialize Stripe client
	stripeClient := stripe.NewClient(stripeConfig.SecretKey, nil)

	// Get the payment intent with expanded fields
	params := &stripe.PaymentIntentRetrieveParams{
		Expand: []*string{
			stripe.String("payment_method"),
			stripe.String("customer"),
		},
	}
	paymentIntent, err := stripeClient.V1PaymentIntents.Retrieve(ctx, paymentIntentID, params)
	if err != nil {
		s.Logger.Errorw("failed to get Stripe payment intent",
			"error", err,
			"payment_intent_id", paymentIntentID)
		return nil, ierr.NewError("failed to get payment status").
			WithHint("Unable to retrieve Stripe payment intent").
			WithReportableDetails(map[string]interface{}{
				"payment_intent_id": paymentIntentID,
				"error":             err.Error(),
			}).
			Mark(ierr.ErrSystem)
	}

	// Log payment intent details for debugging
	s.Logger.Debugw("retrieved Stripe payment intent",
		"payment_intent_id", paymentIntent.ID,
		"status", paymentIntent.Status,
		"has_payment_method", paymentIntent.PaymentMethod != nil,
		"has_customer", paymentIntent.Customer != nil,
	)

	// Extract payment method ID
	var paymentMethodID string
	if paymentIntent.PaymentMethod != nil {
		paymentMethodID = paymentIntent.PaymentMethod.ID
	}

	// Convert amount from cents to decimal
	var amount decimal.Decimal
	if paymentIntent.Amount > 0 {
		amount = decimal.NewFromInt(paymentIntent.Amount).Div(decimal.NewFromInt(100))
	}

	// Get currency
	currency := string(paymentIntent.Currency)
	if currency == "" {
		currency = "usd" // Default to USD
	}

	// Log extracted values for debugging
	s.Logger.Debugw("extracted payment intent status values",
		"payment_intent_id", paymentIntent.ID,
		"status", string(paymentIntent.Status),
		"amount", amount.String(),
		"currency", currency,
		"payment_method_id", paymentMethodID,
		"customer_id", func() string {
			if paymentIntent.Customer != nil {
				return paymentIntent.Customer.ID
			}
			return ""
		}(),
	)

	return &dto.PaymentStatusResponse{
		SessionID:       "", // No session ID for direct payment intent
		PaymentIntentID: paymentIntent.ID,
		PaymentMethodID: paymentMethodID,
		Status:          string(paymentIntent.Status),
		Amount:          amount,
		Currency:        currency,
		CustomerID: func() string {
			if paymentIntent.Customer != nil {
				return paymentIntent.Customer.ID
			}
			return ""
		}(),
		CreatedAt: paymentIntent.Created,
		ExpiresAt: 0, // Payment intents don't have expires_at
		Metadata:  paymentIntent.Metadata,
	}, nil
}

// UpdateStripeCustomerMetadata updates the Stripe customer metadata with FlexPrice information
func (s *StripeService) UpdateStripeCustomerMetadata(ctx context.Context, stripeCustomerID string, cust interface{}) error {
	// Type assertion to get customer data
	var customerID, environmentID, externalID string
	switch customer := cust.(type) {
	case *flexCustomer.Customer:
		customerID = customer.ID
		environmentID = customer.EnvironmentID
		externalID = customer.ExternalID
	default:
		return ierr.NewError("invalid customer type").
			WithHint("Expected customer.Customer type").
			Mark(ierr.ErrValidation)
	}

	// Get Stripe connection
	conn, err := s.ConnectionRepo.GetByProvider(ctx, types.SecretProviderStripe)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to get Stripe connection").
			Mark(ierr.ErrInternal)
	}

	stripeConfig, err := s.GetDecryptedStripeConfig(conn)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to get Stripe configuration").
			Mark(ierr.ErrInternal)
	}

	// Initialize Stripe client
	sc := stripe.NewClient(stripeConfig.SecretKey, nil)

	// Create update parameters
	params := &stripe.CustomerUpdateParams{}
	params.AddMetadata("flexprice_customer_id", customerID)
	params.AddMetadata("flexprice_environment", environmentID)
	params.AddMetadata("external_id", externalID)

	// Update the Stripe customer
	_, err = sc.V1Customers.Update(ctx, stripeCustomerID, params)
	if err != nil {
		s.Logger.Errorw("failed to update Stripe customer metadata",
			"stripe_customer_id", stripeCustomerID,
			"flexprice_customer_id", customerID,
			"error", err)
		return ierr.WithError(err).
			WithHint("Failed to update Stripe customer metadata").
			Mark(ierr.ErrInternal)
	}

	s.Logger.Infow("successfully updated Stripe customer metadata",
		"stripe_customer_id", stripeCustomerID,
		"flexprice_customer_id", customerID)

	return nil
}

// getStripeInvoiceID gets the Stripe invoice ID for a FlexPrice invoice
func (s *StripeService) getStripeInvoiceID(ctx context.Context, invoiceID string) (string, error) {
	mappingService := NewEntityIntegrationMappingService(s.ServiceParams)

	filter := &types.EntityIntegrationMappingFilter{
		EntityID:      invoiceID,
		EntityType:    types.IntegrationEntityTypeInvoice,
		ProviderTypes: []string{"stripe"},
		QueryFilter:   types.NewDefaultQueryFilter(),
	}

	mappings, err := mappingService.GetEntityIntegrationMappings(ctx, filter)
	if err != nil {
		return "", err
	}

	if len(mappings.Items) == 0 {
		return "", ierr.NewError("no Stripe invoice mapping found").
			WithHint("Invoice not synced to Stripe").
			WithReportableDetails(map[string]interface{}{
				"invoice_id": invoiceID,
			}).
			Mark(ierr.ErrNotFound)
	}

	return mappings.Items[0].ProviderEntityID, nil
}

// AttachPaymentToStripeInvoice attaches a successful PaymentIntent to a Stripe invoice
func (s *StripeService) AttachPaymentToStripeInvoice(ctx context.Context, stripeClient *stripe.Client, paymentIntentID, stripeInvoiceID string) error {
	s.Logger.Infow("attaching payment to Stripe invoice",
		"payment_intent_id", paymentIntentID,
		"stripe_invoice_id", stripeInvoiceID)

	// Use the invoice.AttachPayment method as per Stripe documentation
	attachParams := &stripe.InvoiceAttachPaymentParams{
		PaymentIntent: stripe.String(paymentIntentID),
	}

	_, err := stripeClient.V1Invoices.AttachPayment(ctx, stripeInvoiceID, attachParams)
	if err != nil {
		return ierr.NewError("failed to attach payment to Stripe invoice").
			WithHint("Payment succeeded but couldn't be attached to invoice").
			WithReportableDetails(map[string]interface{}{
				"stripe_invoice_id": stripeInvoiceID,
				"payment_intent_id": paymentIntentID,
				"error":             err.Error(),
			}).
			Mark(ierr.ErrSystem)
	}

	s.Logger.Infow("successfully attached payment to Stripe invoice",
		"payment_intent_id", paymentIntentID,
		"stripe_invoice_id", stripeInvoiceID)

	return nil
}

// GetPaymentIntent retrieves a payment intent from Stripe
func (s *StripeService) GetPaymentIntent(ctx context.Context, paymentIntentID, environmentID string) (*stripe.PaymentIntent, error) {
	// Get Stripe connection
	conn, err := s.ConnectionRepo.GetByProvider(ctx, types.SecretProviderStripe)
	if err != nil {
		return nil, ierr.NewError("failed to get Stripe connection").
			WithHint("Stripe connection not configured for this environment").
			Mark(ierr.ErrNotFound)
	}

	stripeConfig, err := s.GetDecryptedStripeConfig(conn)
	if err != nil {
		return nil, ierr.NewError("failed to get Stripe configuration").
			WithHint("Invalid Stripe configuration").
			Mark(ierr.ErrValidation)
	}

	// Initialize Stripe client
	client := stripe.NewClient(stripeConfig.SecretKey, nil)

	// Retrieve the payment intent
	paymentIntent, err := client.V1PaymentIntents.Retrieve(ctx, paymentIntentID, nil)
	if err != nil {
		return nil, err
	}

	return paymentIntent, nil
}

// ProcessExternalStripePayment processes an external Stripe payment and reconciles it with FlexPrice
func (s *StripeService) ProcessExternalStripePayment(ctx context.Context, paymentIntent *stripe.PaymentIntent, stripeInvoiceID string) error {
	// Find the FlexPrice invoice via entity integration mapping
	flexInvoiceID, err := s.getFlexPriceInvoiceID(ctx, stripeInvoiceID)
	if err != nil {
		s.Logger.Errorw("failed to find FlexPrice invoice for Stripe payment",
			"error", err,
			"stripe_invoice_id", stripeInvoiceID,
			"payment_intent_id", paymentIntent.ID)
		return err
	}

	if flexInvoiceID == "" {
		s.Logger.Warnw("no FlexPrice invoice found for external Stripe payment",
			"payment_intent_id", paymentIntent.ID,
			"stripe_invoice_id", stripeInvoiceID)
		return nil // Not an error, just no FlexPrice invoice to sync
	}

	// Process the external payment
	return s.createExternalPaymentRecord(ctx, paymentIntent, flexInvoiceID)
}

// getFlexPriceInvoiceID finds the FlexPrice invoice ID from Stripe invoice ID via entity integration mapping
func (s *StripeService) getFlexPriceInvoiceID(ctx context.Context, stripeInvoiceID string) (string, error) {
	// Create filter to find entity integration mapping for the Stripe invoice
	filter := &types.EntityIntegrationMappingFilter{
		EntityType:        types.IntegrationEntityTypeInvoice,
		ProviderTypes:     []string{string(types.SecretProviderStripe)},
		ProviderEntityIDs: []string{stripeInvoiceID},
	}

	// Get entity integration mappings
	mappings, err := s.EntityIntegrationMappingRepo.List(ctx, filter)
	if err != nil {
		return "", err
	}

	if len(mappings) == 0 {
		return "", nil // No mapping found, not an error
	}

	return mappings[0].EntityID, nil
}

// createExternalPaymentRecord creates a payment record and reconciles the invoice for external Stripe payments
func (s *StripeService) createExternalPaymentRecord(ctx context.Context, paymentIntent *stripe.PaymentIntent, invoiceID string) error {
	// Convert amount from cents to decimal
	amount := decimal.NewFromInt(paymentIntent.Amount).Div(decimal.NewFromInt(100))

	// Get invoice to validate payment amount
	invoiceService := NewInvoiceService(s.ServiceParams)
	invoiceResp, err := invoiceService.GetInvoice(ctx, invoiceID)
	if err != nil {
		s.Logger.Errorw("failed to get invoice for payment validation",
			"error", err,
			"invoice_id", invoiceID,
			"payment_intent_id", paymentIntent.ID)
		return err
	}

	// Check if invoice is already paid
	if invoiceResp.PaymentStatus == types.PaymentStatusSucceeded {
		s.Logger.Infow("invoice is already paid, skipping payment processing",
			"invoice_id", invoiceID,
			"payment_intent_id", paymentIntent.ID,
			"payment_status", invoiceResp.PaymentStatus)
		return nil
	}

	// Validate payment amount doesn't exceed remaining balance
	if amount.GreaterThan(invoiceResp.AmountRemaining) {
		s.Logger.Warnw("payment amount exceeds invoice remaining balance",
			"payment_amount", amount,
			"invoice_remaining", invoiceResp.AmountRemaining,
			"invoice_id", invoiceID,
			"payment_intent_id", paymentIntent.ID)
		// Continue processing but log the warning
	}

	// Create payment record
	paymentService := NewPaymentService(s.ServiceParams)

	// Create as CARD payment with all Stripe details (same as regular Stripe charge)
	gatewayType := types.PaymentGatewayTypeStripe
	createReq := &dto.CreatePaymentRequest{
		DestinationType:   types.PaymentDestinationTypeInvoice,
		DestinationID:     invoiceID,
		PaymentMethodType: types.PaymentMethodTypeCard, // Mark as card payment
		Amount:            amount,
		Currency:          strings.ToUpper(string(paymentIntent.Currency)),
		PaymentGateway:    &gatewayType,
		ProcessPayment:    false, // Don't process - already succeeded in Stripe
		Metadata: types.Metadata{
			"payment_source":        "stripe_external",
			"stripe_payment_intent": paymentIntent.ID,
			"webhook_event_id":      paymentIntent.ID, // For idempotency
		},
	}

	// Add customer ID if available
	if paymentIntent.Customer != nil {
		createReq.Metadata["stripe_customer_id"] = paymentIntent.Customer.ID
	}

	// Add Stripe payment method ID (required for CARD payments)
	if paymentIntent.PaymentMethod != nil {
		createReq.PaymentMethodID = paymentIntent.PaymentMethod.ID
	}

	paymentResp, err := paymentService.CreatePayment(ctx, createReq)
	if err != nil {
		s.Logger.Errorw("failed to create external payment record",
			"error", err,
			"payment_intent_id", paymentIntent.ID,
			"invoice_id", invoiceID)
		return err
	}

	// Update payment to succeeded status with all Stripe details (same as regular Stripe charge)
	now := time.Now().UTC()
	updateReq := dto.UpdatePaymentRequest{
		PaymentStatus:    lo.ToPtr(string(types.PaymentStatusSucceeded)),
		GatewayPaymentID: lo.ToPtr(paymentIntent.ID),
		PaymentGateway:   lo.ToPtr(string(types.PaymentGatewayTypeStripe)),
		SucceededAt:      lo.ToPtr(now),
	}

	// Add payment method ID if available
	if paymentIntent.PaymentMethod != nil {
		updateReq.PaymentMethodID = lo.ToPtr(paymentIntent.PaymentMethod.ID)
	}

	_, err = paymentService.UpdatePayment(ctx, paymentResp.ID, updateReq)
	if err != nil {
		s.Logger.Errorw("failed to update external payment status",
			"error", err,
			"payment_id", paymentResp.ID,
			"payment_intent_id", paymentIntent.ID)
		return err
	}

	s.Logger.Infow("successfully created external payment record",
		"payment_id", paymentResp.ID,
		"payment_intent_id", paymentIntent.ID,
		"invoice_id", invoiceID,
		"amount", amount)

	// Reconcile the invoice with the payment
	if err := s.reconcileInvoiceWithExternalPayment(ctx, invoiceID, amount); err != nil {
		s.Logger.Errorw("failed to reconcile invoice with external payment",
			"error", err,
			"invoice_id", invoiceID,
			"payment_id", paymentResp.ID,
			"amount", amount)
		return err
	}

	return nil
}

// reconcileInvoiceWithExternalPayment updates the invoice amounts and status
func (s *StripeService) reconcileInvoiceWithExternalPayment(ctx context.Context, invoiceID string, paymentAmount decimal.Decimal) error {
	invoiceService := NewInvoiceService(s.ServiceParams)

	// Get the current invoice to calculate proper payment status
	invoiceResp, err := invoiceService.GetInvoice(ctx, invoiceID)
	if err != nil {
		s.Logger.Errorw("failed to get invoice for external payment reconciliation",
			"error", err,
			"invoice_id", invoiceID)
		return err
	}

	// Calculate new amounts
	newAmountPaid := invoiceResp.AmountPaid.Add(paymentAmount)
	newAmountRemaining := invoiceResp.AmountDue.Sub(newAmountPaid)

	// Determine payment status based on remaining amount
	var newPaymentStatus types.PaymentStatus
	if newAmountRemaining.IsZero() {
		newPaymentStatus = types.PaymentStatusSucceeded
	} else if newAmountRemaining.IsNegative() {
		// Invoice is overpaid
		newPaymentStatus = types.PaymentStatusOverpaid
		// For overpaid invoices, amount_remaining should be 0
		newAmountRemaining = decimal.Zero
	} else {
		newPaymentStatus = types.PaymentStatusPending // Partial payment
	}

	s.Logger.Infow("calculated payment status for external payment",
		"invoice_id", invoiceID,
		"payment_amount", paymentAmount,
		"current_amount_paid", invoiceResp.AmountPaid,
		"new_amount_paid", newAmountPaid,
		"amount_due", invoiceResp.AmountDue,
		"new_amount_remaining", newAmountRemaining,
		"new_payment_status", newPaymentStatus)

	// Use the existing ReconcilePaymentStatus method with calculated status
	err = invoiceService.ReconcilePaymentStatus(ctx, invoiceID, newPaymentStatus, &paymentAmount)
	if err != nil {
		s.Logger.Errorw("failed to update invoice payment status",
			"error", err,
			"invoice_id", invoiceID,
			"payment_amount", paymentAmount,
			"payment_status", newPaymentStatus)
		return err
	}

	s.Logger.Infow("successfully reconciled invoice with external payment",
		"invoice_id", invoiceID,
		"payment_amount", paymentAmount,
		"payment_status", newPaymentStatus)

	return nil
}

// SetupIntent creates a Setup Intent with the configured payment provider (generic method)
func (s *StripeService) SetupIntent(ctx context.Context, customerID string, req *dto.CreateSetupIntentRequest) (*dto.SetupIntentResponse, error) {
	// Route to appropriate provider based on request
	switch req.Provider {
	case string(types.PaymentMethodProviderStripe):
		return s.createStripeSetupIntent(ctx, customerID, req)
	default:
		return nil, ierr.NewError("unsupported payment provider").
			WithHint("Currently only 'stripe' provider is supported").
			WithReportableDetails(map[string]interface{}{
				"provider":            req.Provider,
				"supported_providers": []string{"stripe"},
			}).
			Mark(ierr.ErrValidation)
	}
}

// CreateStripeSetupIntent creates a Setup Intent with Stripe Checkout session for saving payment methods
func (s *StripeService) createStripeSetupIntent(ctx context.Context, customerID string, req *dto.CreateSetupIntentRequest) (*dto.SetupIntentResponse, error) {
	s.Logger.Infow("creating stripe setup intent",
		"customer_id", customerID,
		"usage", req.Usage,
		"payment_method_types", req.PaymentMethodTypes,
	)

	// Get Stripe connection for this environment
	conn, err := s.ConnectionRepo.GetByProvider(ctx, types.SecretProviderStripe)
	if err != nil {
		return nil, ierr.NewError("failed to get Stripe connection").
			WithHint("Stripe connection not configured for this environment").
			Mark(ierr.ErrNotFound)
	}

	// Debug logging for context
	s.Logger.Infow("setup intent context debug",
		"customer_id", customerID,
		"tenant_id", types.GetTenantID(ctx),
		"environment_id", types.GetEnvironmentID(ctx),
	)

	// Ensure customer is synced to Stripe before creating setup intent
	customerResp, err := s.EnsureCustomerSyncedToStripe(ctx, customerID)
	if err != nil {
		s.Logger.Errorw("failed to sync customer to Stripe",
			"error", err,
			"customer_id", customerID,
			"tenant_id", types.GetTenantID(ctx),
			"environment_id", types.GetEnvironmentID(ctx),
		)
		return nil, ierr.WithError(err).
			WithHint("Failed to sync customer to Stripe").
			WithReportableDetails(map[string]interface{}{
				"customer_id": customerID,
			}).
			Mark(ierr.ErrValidation)
	}

	// Get Stripe customer ID (should exist after sync)
	stripeCustomerID, exists := customerResp.Customer.Metadata["stripe_customer_id"]
	if !exists || stripeCustomerID == "" {
		return nil, ierr.NewError("customer does not have Stripe customer ID after sync").
			WithHint("Failed to sync customer to Stripe").
			WithReportableDetails(map[string]interface{}{
				"customer_id": customerID,
			}).
			Mark(ierr.ErrValidation)
	}

	// Get Stripe configuration
	stripeConfig, err := s.GetDecryptedStripeConfig(conn)
	if err != nil {
		return nil, ierr.NewError("failed to get Stripe configuration").
			WithHint("Invalid Stripe configuration").
			Mark(ierr.ErrValidation)
	}

	// Initialize Stripe client
	stripeClient := stripe.NewClient(stripeConfig.SecretKey, nil)

	// Set default values
	usage := req.Usage
	if usage == "" {
		usage = "off_session" // Default as per Stripe documentation
	}

	paymentMethodTypes := req.PaymentMethodTypes
	// If no payment method types specified, let Stripe use its defaults (supports all available types)

	// Use user-provided URLs directly (no defaults)
	successURL := req.SuccessURL
	cancelURL := req.CancelURL

	// Build metadata for the setup intent
	metadata := map[string]string{
		"customer_id":    customerID,
		"environment_id": types.GetEnvironmentID(ctx),
		"usage":          usage,
	}

	// Add custom metadata if provided
	for k, v := range req.Metadata {
		metadata[k] = v
	}

	// Add set_default flag to metadata if requested
	if req.SetDefault {
		metadata["set_default"] = "true"
		s.Logger.Infow("setup intent will be set as default when succeeded",
			"customer_id", customerID,
			"set_default", "true",
			"metadata", metadata)
	}

	// Create Setup Intent first
	setupIntentParams := &stripe.SetupIntentCreateParams{
		Customer: stripe.String(stripeCustomerID),
		Usage:    stripe.String(usage),
		Metadata: metadata,
	}

	// Add payment method types if specified (if empty, Stripe will use all available types)
	if len(paymentMethodTypes) > 0 {
		for _, pmType := range paymentMethodTypes {
			setupIntentParams.PaymentMethodTypes = append(setupIntentParams.PaymentMethodTypes, stripe.String(pmType))
		}
	}

	setupIntent, err := stripeClient.V1SetupIntents.Create(ctx, setupIntentParams)
	if err != nil {
		s.Logger.Errorw("failed to create Stripe setup intent",
			"error", err,
			"customer_id", customerID)
		return nil, ierr.NewError("failed to create setup intent").
			WithHint("Unable to create Stripe setup intent").
			WithReportableDetails(map[string]interface{}{
				"customer_id": customerID,
				"error":       err.Error(),
			}).
			Mark(ierr.ErrSystem)
	}

	s.Logger.Infow("created stripe setup intent",
		"setup_intent_id", setupIntent.ID,
		"customer_id", customerID,
		"usage", usage,
	)

	// Create Checkout Session in setup mode
	checkoutParams := &stripe.CheckoutSessionCreateParams{
		Mode:       stripe.String("setup"),
		Customer:   stripe.String(stripeCustomerID),
		Currency:   stripe.String("usd"), // Required by Stripe even for setup mode
		SuccessURL: stripe.String(successURL),
		CancelURL:  stripe.String(cancelURL),
		Metadata:   metadata,
	}

	// Add payment method types if specified (if empty, Stripe will use all available types)
	if len(paymentMethodTypes) > 0 {
		var pmTypes []*string
		for _, pmType := range paymentMethodTypes {
			pmTypes = append(pmTypes, stripe.String(pmType))
		}
		checkoutParams.PaymentMethodTypes = pmTypes
	}

	// Link the setup intent to the checkout session
	checkoutParams.SetupIntentData = &stripe.CheckoutSessionCreateSetupIntentDataParams{
		Metadata: metadata,
	}

	checkoutSession, err := stripeClient.V1CheckoutSessions.Create(ctx, checkoutParams)
	if err != nil {
		s.Logger.Errorw("failed to create Stripe checkout session for setup intent",
			"error", err,
			"setup_intent_id", setupIntent.ID,
			"customer_id", customerID)
		return nil, ierr.NewError("failed to create setup intent checkout session").
			WithHint("Unable to create Stripe checkout session").
			WithReportableDetails(map[string]interface{}{
				"setup_intent_id": setupIntent.ID,
				"customer_id":     customerID,
				"error":           err.Error(),
			}).
			Mark(ierr.ErrSystem)
	}

	response := &dto.SetupIntentResponse{
		SetupIntentID:     setupIntent.ID,
		CheckoutSessionID: checkoutSession.ID,
		CheckoutURL:       checkoutSession.URL,
		ClientSecret:      setupIntent.ClientSecret,
		Status:            string(setupIntent.Status),
		Usage:             usage,
		CustomerID:        customerID,
		CreatedAt:         setupIntent.Created,
		ExpiresAt:         checkoutSession.ExpiresAt,
	}

	s.Logger.Infow("successfully created stripe setup intent with checkout session",
		"setup_intent_id", setupIntent.ID,
		"checkout_session_id", checkoutSession.ID,
		"checkout_url", checkoutSession.URL,
		"customer_id", customerID,
		"usage", usage,
	)

	return response, nil
}

// getPaymentMethodDetails retrieves payment method details from Stripe
func (s *StripeService) getPaymentMethodDetails(ctx context.Context, stripeClient *stripe.Client, paymentMethodID string) (*dto.PaymentMethodResponse, error) {
	paymentMethod, err := stripeClient.V1PaymentMethods.Retrieve(ctx, paymentMethodID, nil)
	if err != nil {
		return nil, err
	}

	response := &dto.PaymentMethodResponse{
		ID:   paymentMethod.ID,
		Type: string(paymentMethod.Type),
		Customer: func() string {
			if paymentMethod.Customer != nil {
				return paymentMethod.Customer.ID
			}
			return ""
		}(),
		Created:  paymentMethod.Created,
		Metadata: make(map[string]interface{}),
	}

	// Convert metadata
	for k, v := range paymentMethod.Metadata {
		response.Metadata[k] = v
	}

	// Add card details if it's a card
	if paymentMethod.Type == stripe.PaymentMethodTypeCard && paymentMethod.Card != nil {
		response.Card = &dto.CardDetails{
			Brand:       string(paymentMethod.Card.Brand),
			Last4:       paymentMethod.Card.Last4,
			ExpMonth:    int(paymentMethod.Card.ExpMonth),
			ExpYear:     int(paymentMethod.Card.ExpYear),
			Fingerprint: paymentMethod.Card.Fingerprint,
		}
	}

	return response, nil
}

// ListCustomerPaymentMethods lists only the successfully saved payment methods for a customer (generic method)
func (s *StripeService) ListCustomerPaymentMethods(ctx context.Context, customerID string, req *dto.ListPaymentMethodsRequest) (*dto.MultiProviderPaymentMethodsResponse, error) {
	response := &dto.MultiProviderPaymentMethodsResponse{}

	// Route to appropriate provider based on request
	switch req.Provider {
	case string(types.PaymentMethodProviderStripe):
		stripePaymentMethods, err := s.listStripeCustomerPaymentMethods(ctx, customerID, req)
		if err != nil {
			s.Logger.Errorw("failed to get Stripe payment methods", "error", err, "customer_id", customerID)
			return nil, err
		}
		response.Stripe = stripePaymentMethods.Data

	default:
		return nil, ierr.NewError("unsupported payment provider").
			WithHint("Currently only 'stripe' provider is supported").
			WithReportableDetails(map[string]interface{}{
				"provider":            req.Provider,
				"supported_providers": []types.PaymentMethodProvider{types.PaymentMethodProviderStripe},
			}).
			Mark(ierr.ErrValidation)
	}

	return response, nil
}

// listStripeCustomerPaymentMethods lists only successfully saved payment methods using Stripe's Customer.ListPaymentMethods
func (s *StripeService) listStripeCustomerPaymentMethods(ctx context.Context, customerID string, req *dto.ListPaymentMethodsRequest) (*dto.ListSetupIntentsResponse, error) {
	s.Logger.Infow("listing stripe customer payment methods",
		"customer_id", customerID,
		"limit", req.Limit,
	)

	// Get Stripe connection for this environment
	conn, err := s.ConnectionRepo.GetByProvider(ctx, types.SecretProviderStripe)
	if err != nil {
		return nil, ierr.NewError("failed to get Stripe connection").
			WithHint("Stripe connection not configured for this environment").
			Mark(ierr.ErrNotFound)
	}

	// Ensure customer is synced to Stripe
	customerResp, err := s.EnsureCustomerSyncedToStripe(ctx, customerID)
	if err != nil {
		s.Logger.Errorw("failed to sync customer to Stripe",
			"error", err,
			"customer_id", customerID,
		)
		return nil, ierr.WithError(err).
			WithHint("Failed to sync customer to Stripe").
			WithReportableDetails(map[string]interface{}{
				"customer_id": customerID,
			}).
			Mark(ierr.ErrValidation)
	}

	// Get Stripe customer ID
	stripeCustomerID, exists := customerResp.Customer.Metadata["stripe_customer_id"]
	if !exists || stripeCustomerID == "" {
		return nil, ierr.NewError("customer does not have Stripe customer ID after sync").
			WithHint("Failed to sync customer to Stripe").
			WithReportableDetails(map[string]interface{}{
				"customer_id": customerID,
			}).
			Mark(ierr.ErrValidation)
	}

	// Get Stripe configuration
	stripeConfig, err := s.GetDecryptedStripeConfig(conn)
	if err != nil {
		return nil, ierr.NewError("failed to get Stripe configuration").
			WithHint("Invalid Stripe configuration").
			Mark(ierr.ErrValidation)
	}

	// Initialize Stripe client
	stripeClient := stripe.NewClient(stripeConfig.SecretKey, nil)

	// Get customer's default payment method ID
	var defaultPaymentMethodID string
	customer, err := stripeClient.V1Customers.Retrieve(ctx, stripeCustomerID, nil)
	if err == nil && customer.InvoiceSettings != nil && customer.InvoiceSettings.DefaultPaymentMethod != nil {
		defaultPaymentMethodID = customer.InvoiceSettings.DefaultPaymentMethod.ID
		s.Logger.Infow("found default payment method for customer",
			"customer_id", customerID,
			"default_payment_method_id", defaultPaymentMethodID)
	}

	// Set default limit if not provided
	limit := req.Limit
	if limit <= 0 {
		limit = 10 // Default limit
	}
	if limit > 100 {
		limit = 100 // Max limit
	}

	// Build Stripe API parameters for listing payment methods
	params := &stripe.PaymentMethodListParams{
		Customer: stripe.String(stripeCustomerID),
		// No Type restriction - list ALL payment method types (card, us_bank_account, sepa_debit, etc.)
	}
	params.Limit = stripe.Int64(int64(limit))

	// Add pagination parameters
	if req.StartingAfter != "" {
		params.StartingAfter = stripe.String(req.StartingAfter)
	}
	if req.EndingBefore != "" {
		params.EndingBefore = stripe.String(req.EndingBefore)
	}

	// List Payment Methods from Stripe (only successfully saved ones)
	paymentMethodsList := stripeClient.V1PaymentMethods.List(ctx, params)

	var setupIntents []dto.SetupIntentListItem
	var hasMore bool
	var totalCount int

	// Iterate through the payment methods
	paymentMethodsList(func(pm *stripe.PaymentMethod, err error) bool {
		if err != nil {
			s.Logger.Errorw("failed to iterate payment methods",
				"error", err,
				"customer_id", customerID,
				"stripe_customer_id", stripeCustomerID)
			return false // Stop iteration on error
		}

		// Create a simplified setup intent item representing the saved payment method
		item := dto.SetupIntentListItem{
			ID:              pm.ID,         // Use payment method ID as the main ID
			Status:          "succeeded",   // All listed payment methods are successfully saved
			Usage:           "off_session", // Payment methods are typically for off-session use
			CustomerID:      customerID,
			PaymentMethodID: pm.ID,
			IsDefault:       pm.ID == defaultPaymentMethodID, // Mark if this is the default payment method
			CreatedAt:       pm.Created,
		}

		// Add payment method details
		pmDetails := &dto.PaymentMethodResponse{
			ID:   pm.ID,
			Type: string(pm.Type),
			Customer: func() string {
				if pm.Customer != nil {
					return pm.Customer.ID
				}
				return stripeCustomerID
			}(),
			Created:  pm.Created,
			Metadata: make(map[string]interface{}),
		}

		// Convert metadata
		for k, v := range pm.Metadata {
			pmDetails.Metadata[k] = v
		}

		// Add card details if it's a card
		if pm.Type == stripe.PaymentMethodTypeCard && pm.Card != nil {
			pmDetails.Card = &dto.CardDetails{
				Brand:       string(pm.Card.Brand),
				Last4:       pm.Card.Last4,
				ExpMonth:    int(pm.Card.ExpMonth),
				ExpYear:     int(pm.Card.ExpYear),
				Fingerprint: pm.Card.Fingerprint,
			}
		}

		item.PaymentMethodDetails = pmDetails
		setupIntents = append(setupIntents, item)
		totalCount++
		return true // Continue iteration
	})

	// Check if there are more results
	if len(setupIntents) == limit {
		hasMore = true
	}

	response := &dto.ListSetupIntentsResponse{
		Data:       setupIntents,
		HasMore:    hasMore,
		TotalCount: totalCount,
	}

	s.Logger.Infow("successfully listed stripe customer payment methods",
		"customer_id", customerID,
		"stripe_customer_id", stripeCustomerID,
		"count", len(setupIntents),
		"has_more", hasMore,
	)

	return response, nil
}

// IsInvoiceSyncedToStripe checks if an invoice is synced to Stripe by looking up entity integration mapping
func (s *StripeService) IsInvoiceSyncedToStripe(ctx context.Context, invoiceID string) bool {
	filter := &types.EntityIntegrationMappingFilter{
		EntityID:      invoiceID,
		EntityType:    types.IntegrationEntityTypeInvoice,
		ProviderTypes: []string{"stripe"},
		QueryFilter:   types.NewDefaultQueryFilter(),
	}

	mappings, err := s.EntityIntegrationMappingRepo.List(ctx, filter)
	if err != nil {
		s.Logger.Errorw("failed to check invoice Stripe sync status",
			"error", err,
			"invoice_id", invoiceID,
		)
		// In case of error, assume not synced to be safe
		return false
	}

	// If we have any mappings, the invoice is synced to Stripe
	return len(mappings) > 0
}
