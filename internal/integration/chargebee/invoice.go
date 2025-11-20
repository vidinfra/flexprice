package chargebee

import (
	"context"
	"time"

	"github.com/chargebee/chargebee-go/v3/enum"
	chargebeeInvoice "github.com/chargebee/chargebee-go/v3/models/invoice"
	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/entityintegrationmapping"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/interfaces"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// ChargebeeInvoiceService defines the interface for Chargebee invoice operations
type ChargebeeInvoiceService interface {
	CreateInvoice(ctx context.Context, req *InvoiceCreateRequest) (*InvoiceResponse, error)
	RetrieveInvoice(ctx context.Context, invoiceID string) (*InvoiceResponse, error)
	SyncInvoiceToChargebee(ctx context.Context, req ChargebeeInvoiceSyncRequest, customerService interfaces.CustomerService) (*ChargebeeInvoiceSyncResponse, error)
}

// InvoiceService handles Chargebee invoice operations
type InvoiceService struct {
	client                       ChargebeeClient
	customerSvc                  ChargebeeCustomerService
	invoiceRepo                  invoice.Repository
	entityIntegrationMappingRepo entityintegrationmapping.Repository
	logger                       *logger.Logger
}

// NewInvoiceService creates a new Chargebee invoice service
func NewInvoiceService(
	client ChargebeeClient,
	customerSvc ChargebeeCustomerService,
	invoiceRepo invoice.Repository,
	entityIntegrationMappingRepo entityintegrationmapping.Repository,
	logger *logger.Logger,
) ChargebeeInvoiceService {
	return &InvoiceService{
		client:                       client,
		customerSvc:                  customerSvc,
		invoiceRepo:                  invoiceRepo,
		entityIntegrationMappingRepo: entityIntegrationMappingRepo,
		logger:                       logger,
	}
}

// ChargebeeInvoiceSyncRequest represents a request to sync an invoice to Chargebee
type ChargebeeInvoiceSyncRequest struct {
	InvoiceID string `json:"invoice_id" validate:"required"`
}

// ChargebeeInvoiceSyncResponse represents the response from syncing an invoice to Chargebee
type ChargebeeInvoiceSyncResponse struct {
	ChargebeeInvoiceID string `json:"chargebee_invoice_id"`
	Status             string `json:"status"`
	Total              int64  `json:"total"`
	AmountDue          int64  `json:"amount_due"`
	CurrencyCode       string `json:"currency_code"`
}

// CreateInvoice creates a new invoice in Chargebee using charge items
func (s *InvoiceService) CreateInvoice(ctx context.Context, req *InvoiceCreateRequest) (*InvoiceResponse, error) {
	s.logger.Infow("creating invoice in Chargebee",
		"customer_id", req.CustomerID,
		"auto_collection", req.AutoCollection,
		"line_items_count", len(req.LineItems))

	// Prepare request params
	createParams := &chargebeeInvoice.CreateForChargeItemsAndChargesRequestParams{
		CustomerId:     req.CustomerID,
		AutoCollection: enum.AutoCollection(req.AutoCollection),
	}

	// Add item prices (line items)
	if len(req.LineItems) > 0 {
		itemPrices := make([]*chargebeeInvoice.CreateForChargeItemsAndChargesItemPriceParams, 0, len(req.LineItems))
		for _, item := range req.LineItems {
			itemPrice := &chargebeeInvoice.CreateForChargeItemsAndChargesItemPriceParams{
				ItemPriceId: item.ItemPriceID,
				Quantity:    lo.ToPtr(int32(item.Quantity)),
			}

			if item.UnitAmount > 0 {
				itemPrice.UnitPrice = lo.ToPtr(item.UnitAmount)
			}

			if item.DateFrom != nil {
				itemPrice.DateFrom = lo.ToPtr(item.DateFrom.Unix())
			}

			if item.DateTo != nil {
				itemPrice.DateTo = lo.ToPtr(item.DateTo.Unix())
			}

			itemPrices = append(itemPrices, itemPrice)
		}
		createParams.ItemPrices = itemPrices
	}

	// Add optional dates
	if req.Date != nil {
		createParams.InvoiceDate = lo.ToPtr(req.Date.Unix())
	}

	// Create invoice using client wrapper
	result, err := s.client.CreateInvoice(ctx, createParams)
	if err != nil {
		s.logger.Errorw("failed to create invoice in Chargebee",
			"customer_id", req.CustomerID,
			"error", err)
		return nil, ierr.NewError("failed to create invoice in Chargebee").
			WithReportableDetails(map[string]interface{}{
				"error":       err.Error(),
				"customer_id": req.CustomerID,
			}).
			WithHint("Check Chargebee API credentials and invoice data").
			Mark(ierr.ErrValidation)
	}

	invoiceData := result.Invoice

	s.logger.Infow("successfully created invoice in Chargebee",
		"invoice_id", invoiceData.Id,
		"customer_id", invoiceData.CustomerId,
		"status", invoiceData.Status,
		"total", invoiceData.Total)

	// Convert to our DTO format
	invoiceResponse := &InvoiceResponse{
		ID:              invoiceData.Id,
		CustomerID:      invoiceData.CustomerId,
		Status:          string(invoiceData.Status),
		AutoCollection:  req.AutoCollection, // Use the request value since API doesn't return it
		Total:           invoiceData.Total,
		AmountDue:       invoiceData.AmountDue,
		AmountPaid:      invoiceData.AmountPaid,
		CurrencyCode:    invoiceData.CurrencyCode,
		Date:            time.Unix(invoiceData.Date, 0),
		CreatedAt:       time.Unix(invoiceData.GeneratedAt, 0), // Use GeneratedAt as CreatedAt
		UpdatedAt:       time.Unix(invoiceData.UpdatedAt, 0),
		ResourceVersion: invoiceData.ResourceVersion,
	}

	if invoiceData.DueDate > 0 {
		dueDate := time.Unix(invoiceData.DueDate, 0)
		invoiceResponse.DueDate = &dueDate
	}

	// Convert line items if present
	if len(invoiceData.LineItems) > 0 {
		lineItems := make([]InvoiceLineItemResponse, 0, len(invoiceData.LineItems))
		for _, item := range invoiceData.LineItems {
			lineItem := InvoiceLineItemResponse{
				ID:          item.Id,
				ItemPriceID: item.EntityId, // EntityId contains the item price ID
				EntityType:  string(item.EntityType),
				Quantity:    int(item.Quantity),
				UnitAmount:  item.UnitAmount,
				Amount:      item.Amount,
				Description: item.Description,
				DateFrom:    time.Unix(item.DateFrom, 0),
				DateTo:      time.Unix(item.DateTo, 0),
			}
			lineItems = append(lineItems, lineItem)
		}
		invoiceResponse.LineItems = lineItems
	}

	return invoiceResponse, nil
}

// SyncInvoiceToChargebee syncs a FlexPrice invoice to Chargebee
func (s *InvoiceService) SyncInvoiceToChargebee(
	ctx context.Context,
	req ChargebeeInvoiceSyncRequest,
	customerService interfaces.CustomerService,
) (*ChargebeeInvoiceSyncResponse, error) {
	s.logger.Infow("starting Chargebee invoice sync",
		"invoice_id", req.InvoiceID)

	// Step 1: Check if Chargebee connection exists
	if !s.client.HasChargebeeConnection(ctx) {
		return nil, ierr.NewError("Chargebee connection not available").
			WithHint("Chargebee integration must be configured for invoice sync").
			Mark(ierr.ErrNotFound)
	}

	// Step 2: Get FlexPrice invoice
	flexInvoice, err := s.invoiceRepo.Get(ctx, req.InvoiceID)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to get FlexPrice invoice").
			Mark(ierr.ErrDatabase)
	}

	// Step 3: Check if invoice is already synced to avoid duplicates
	existingMapping, err := s.getExistingChargebeeMapping(ctx, req.InvoiceID)
	if err != nil && !ierr.IsNotFound(err) {
		return nil, err
	}

	var chargebeeInvoiceID string
	if existingMapping != nil {
		chargebeeInvoiceID = existingMapping.ProviderEntityID
		s.logger.Infow("invoice already synced to Chargebee",
			"invoice_id", req.InvoiceID,
			"chargebee_invoice_id", chargebeeInvoiceID)
		// Fetch existing invoice details and return
		invoiceResp, err := s.RetrieveInvoice(ctx, chargebeeInvoiceID)
		if err != nil {
			return nil, err
		}
		return &ChargebeeInvoiceSyncResponse{
			ChargebeeInvoiceID: invoiceResp.ID,
			Status:             invoiceResp.Status,
			Total:              invoiceResp.Total,
			AmountDue:          invoiceResp.AmountDue,
			CurrencyCode:       invoiceResp.CurrencyCode,
		}, nil
	}

	// Step 4: Ensure customer is synced to Chargebee
	flexpriceCustomer, err := s.customerSvc.EnsureCustomerSyncedToChargebee(ctx, flexInvoice.CustomerID, customerService)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to sync customer to Chargebee").
			Mark(ierr.ErrInternal)
	}

	chargebeeCustomerID := flexpriceCustomer.Metadata["chargebee_customer_id"]
	if chargebeeCustomerID == "" {
		// Try to get from entity mapping if not in metadata
		chargebeeCustomerID, err = s.customerSvc.GetChargebeeCustomerID(ctx, flexInvoice.CustomerID)
		if err != nil {
			return nil, ierr.WithError(err).
				WithHint("Failed to get Chargebee customer ID").
				Mark(ierr.ErrInternal)
		}
	}

	s.logger.Infow("customer synced to Chargebee",
		"customer_id", flexInvoice.CustomerID,
		"chargebee_customer_id", chargebeeCustomerID)

	// Step 5: Check if customer has payment method and set auto_collection accordingly
	hasPaymentMethod, err := s.customerHasPaymentMethod(ctx, chargebeeCustomerID)
	if err != nil {
		s.logger.Warnw("failed to check customer payment method, defaulting to auto_collection off",
			"error", err,
			"chargebee_customer_id", chargebeeCustomerID)
		hasPaymentMethod = false
	}

	autoCollection := "off"
	if hasPaymentMethod {
		autoCollection = "on"
	}

	s.logger.Infow("determined auto_collection setting",
		"chargebee_customer_id", chargebeeCustomerID,
		"has_payment_method", hasPaymentMethod,
		"auto_collection", autoCollection)

	// Step 6: Build line items with Chargebee item price IDs
	lineItems, err := s.buildLineItems(ctx, flexInvoice)
	if err != nil {
		return nil, err
	}

	if len(lineItems) == 0 {
		return nil, ierr.NewError("invoice has no line items").
			WithHint("Cannot create Chargebee invoice without line items").
			Mark(ierr.ErrValidation)
	}

	// Step 7: Create invoice in Chargebee
	invoiceReq := &InvoiceCreateRequest{
		CustomerID:     chargebeeCustomerID,
		AutoCollection: autoCollection,
		LineItems:      lineItems,
	}

	// Set invoice date if available (use FinalizedAt or CreatedAt)
	if flexInvoice.FinalizedAt != nil {
		invoiceReq.Date = flexInvoice.FinalizedAt
	} else {
		// Use CreatedAt as fallback
		createdAt := flexInvoice.CreatedAt
		invoiceReq.Date = &createdAt
	}

	chargebeeInvoice, err := s.CreateInvoice(ctx, invoiceReq)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to create invoice in Chargebee").
			Mark(ierr.ErrInternal)
	}

	chargebeeInvoiceID = chargebeeInvoice.ID
	s.logger.Infow("successfully created invoice in Chargebee",
		"invoice_id", req.InvoiceID,
		"chargebee_invoice_id", chargebeeInvoiceID)

	// Step 8: Create entity integration mapping
	if err := s.createInvoiceMapping(ctx, req.InvoiceID, chargebeeInvoiceID, flexInvoice.EnvironmentID); err != nil {
		s.logger.Errorw("failed to create invoice mapping",
			"error", err,
			"invoice_id", req.InvoiceID,
			"chargebee_invoice_id", chargebeeInvoiceID)
		// Don't fail the sync, just log the error
	}

	// Step 9: Build and return response
	return &ChargebeeInvoiceSyncResponse{
		ChargebeeInvoiceID: chargebeeInvoice.ID,
		Status:             chargebeeInvoice.Status,
		Total:              chargebeeInvoice.Total,
		AmountDue:          chargebeeInvoice.AmountDue,
		CurrencyCode:       chargebeeInvoice.CurrencyCode,
	}, nil
}

// buildLineItems converts FlexPrice line items to Chargebee format
// Maps FlexPrice price IDs to Chargebee item price IDs using entity mapping
func (s *InvoiceService) buildLineItems(ctx context.Context, flexInvoice *invoice.Invoice) ([]InvoiceLineItem, error) {
	lineItems := make([]InvoiceLineItem, 0)

	for _, item := range flexInvoice.LineItems {
		// Skip zero-amount items
		if item.Amount.IsZero() {
			s.logger.Debugw("skipping zero-amount line item",
				"invoice_id", flexInvoice.ID,
				"line_item_id", item.ID)
			continue
		}

		// Skip if price ID is not available
		if item.PriceID == nil || *item.PriceID == "" {
			s.logger.Debugw("line item has no price ID, skipping",
				"invoice_id", flexInvoice.ID,
				"line_item_id", item.ID)
			continue
		}

		// Get Chargebee item price ID AND check if it's tiered
		chargebeeItemPriceID, isTiered, err := s.getChargebeeItemPriceIDAndCheckTiered(ctx, *item.PriceID)
		if err != nil {
			s.logger.Errorw("failed to get Chargebee item price ID for line item",
				"invoice_id", flexInvoice.ID,
				"line_item_id", item.ID,
				"price_id", *item.PriceID,
				"error", err)
			// Skip this line item if we can't find the mapping
			continue
		}

		lineItem := InvoiceLineItem{
			ItemPriceID: chargebeeItemPriceID,
		}

		// CRITICAL: Different handling based on pricing model
		if isTiered {
			// For tiered/volume/stairstep pricing: Send ONLY quantity, let Chargebee calculate
			// NOTE: Chargebee's calculation may differ slightly due to tier precision rounding
			lineItem.Quantity = int(item.Quantity.IntPart())
			// DO NOT set UnitAmount - Chargebee will reject it
			s.logger.Debugw("tiered price line item - using quantity only",
				"item_price_id", chargebeeItemPriceID,
				"quantity", lineItem.Quantity,
				"flexprice_amount", item.Amount.String())
		} else {
			// For flat_fee/per_unit/package pricing: Use Quantity=1 + exact amount for precision
			// This ensures exact amount matching with FlexPrice's calculation
			lineItem.Quantity = 1
			lineItem.UnitAmount = convertAmountToSmallestUnit(item.Amount.InexactFloat64(), flexInvoice.Currency)
			s.logger.Debugw("non-tiered price line item - using exact amount",
				"item_price_id", chargebeeItemPriceID,
				"quantity", 1,
				"unit_amount", lineItem.UnitAmount,
				"flexprice_amount", item.Amount.String())
		}

		// Add description if available (use DisplayName or PlanDisplayName)
		if item.DisplayName != nil && *item.DisplayName != "" {
			lineItem.Description = *item.DisplayName
		} else if item.PlanDisplayName != nil && *item.PlanDisplayName != "" {
			lineItem.Description = *item.PlanDisplayName
		}

		// Add date range if available
		if item.PeriodStart != nil {
			lineItem.DateFrom = item.PeriodStart
		}
		if item.PeriodEnd != nil {
			lineItem.DateTo = item.PeriodEnd
		}

		lineItems = append(lineItems, lineItem)
	}

	return lineItems, nil
}

// getChargebeeItemPriceIDAndCheckTiered retrieves the Chargebee item price ID and checks if it uses tiered pricing
func (s *InvoiceService) getChargebeeItemPriceIDAndCheckTiered(ctx context.Context, flexPriceID string) (string, bool, error) {
	if flexPriceID == "" {
		return "", false, ierr.NewError("price ID is required").
			WithHint("Line item must have a price ID").
			Mark(ierr.ErrValidation)
	}

	// Query entity mapping table for price ID
	filter := types.NewEntityIntegrationMappingFilter()
	filter.EntityID = flexPriceID
	filter.EntityType = types.IntegrationEntityTypeItemPrice
	filter.ProviderTypes = []string{string(types.SecretProviderChargebee)}

	mappings, err := s.entityIntegrationMappingRepo.List(ctx, filter)
	if err != nil {
		return "", false, ierr.WithError(err).
			WithHint("Failed to get Chargebee item price mapping").
			Mark(ierr.ErrDatabase)
	}

	if len(mappings) == 0 {
		return "", false, ierr.NewError("Chargebee item price not found for FlexPrice price").
			WithHint("Price must be synced to Chargebee before creating invoice").
			WithReportableDetails(map[string]interface{}{
				"flexprice_price_id": flexPriceID,
			}).
			Mark(ierr.ErrNotFound)
	}

	chargebeeItemPriceID := mappings[0].ProviderEntityID

	// Fetch the item price from Chargebee to check its pricing model
	result, err := s.client.RetrieveItemPrice(ctx, chargebeeItemPriceID)
	if err != nil {
		s.logger.Warnw("failed to retrieve item price from Chargebee, assuming flat_fee",
			"item_price_id", chargebeeItemPriceID,
			"error", err)
		// Fallback: assume flat_fee if we can't fetch
		return chargebeeItemPriceID, false, nil
	}

	pricingModel := string(result.ItemPrice.PricingModel)
	isTiered := pricingModel == "tiered" || pricingModel == "volume" || pricingModel == "stairstep"

	s.logger.Debugw("retrieved pricing model for item price",
		"item_price_id", chargebeeItemPriceID,
		"pricing_model", pricingModel,
		"is_tiered", isTiered)

	return chargebeeItemPriceID, isTiered, nil
}

// getChargebeeItemPriceIDSimple retrieves the Chargebee item price ID from entity mapping
func (s *InvoiceService) getChargebeeItemPriceIDSimple(ctx context.Context, flexPriceID string) (string, error) {
	if flexPriceID == "" {
		return "", ierr.NewError("price ID is required").
			WithHint("Line item must have a price ID").
			Mark(ierr.ErrValidation)
	}

	// Query entity mapping table for price ID
	filter := types.NewEntityIntegrationMappingFilter()
	filter.EntityID = flexPriceID
	filter.EntityType = types.IntegrationEntityTypeItemPrice
	filter.ProviderTypes = []string{string(types.SecretProviderChargebee)}

	mappings, err := s.entityIntegrationMappingRepo.List(ctx, filter)
	if err != nil {
		return "", ierr.WithError(err).
			WithHint("Failed to get Chargebee item price mapping").
			Mark(ierr.ErrDatabase)
	}

	if len(mappings) == 0 {
		return "", ierr.NewError("Chargebee item price not found for FlexPrice price").
			WithHint("Price must be synced to Chargebee before creating invoice").
			WithReportableDetails(map[string]interface{}{
				"flexprice_price_id": flexPriceID,
			}).
			Mark(ierr.ErrNotFound)
	}

	return mappings[0].ProviderEntityID, nil
}

// getExistingChargebeeMapping checks if invoice is already synced to Chargebee
func (s *InvoiceService) getExistingChargebeeMapping(ctx context.Context, invoiceID string) (*entityintegrationmapping.EntityIntegrationMapping, error) {
	filter := types.NewEntityIntegrationMappingFilter()
	filter.EntityID = invoiceID
	filter.EntityType = types.IntegrationEntityTypeInvoice
	filter.ProviderTypes = []string{string(types.SecretProviderChargebee)}

	mappings, err := s.entityIntegrationMappingRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	if len(mappings) == 0 {
		return nil, ierr.NewError("invoice mapping not found").
			Mark(ierr.ErrNotFound)
	}

	return mappings[0], nil
}

// createInvoiceMapping creates an entity integration mapping for the invoice
func (s *InvoiceService) createInvoiceMapping(ctx context.Context, invoiceID, chargebeeInvoiceID, environmentID string) error {
	mapping := &entityintegrationmapping.EntityIntegrationMapping{
		ID:               types.GenerateUUIDWithPrefix(types.UUID_PREFIX_ENTITY_INTEGRATION_MAPPING),
		EntityType:       types.IntegrationEntityTypeInvoice,
		EntityID:         invoiceID,
		ProviderType:     string(types.SecretProviderChargebee),
		ProviderEntityID: chargebeeInvoiceID,
		EnvironmentID:    environmentID,
		BaseModel:        types.GetDefaultBaseModel(ctx),
	}

	// Get tenant_id from context or invoice
	// We'll need to get it from the invoice entity
	inv, err := s.invoiceRepo.Get(ctx, invoiceID)
	if err == nil {
		mapping.TenantID = inv.TenantID
	}

	err = s.entityIntegrationMappingRepo.Create(ctx, mapping)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to create invoice mapping").
			Mark(ierr.ErrDatabase)
	}

	return nil
}

// customerHasPaymentMethod checks if a Chargebee customer has a payment method
func (s *InvoiceService) customerHasPaymentMethod(ctx context.Context, chargebeeCustomerID string) (bool, error) {
	// Retrieve customer from Chargebee using client wrapper
	result, err := s.client.RetrieveCustomer(ctx, chargebeeCustomerID)
	if err != nil {
		return false, ierr.WithError(err).
			WithHint("Failed to retrieve customer from Chargebee").
			Mark(ierr.ErrInternal)
	}

	customer := result.Customer

	// Check if customer has a primary payment source or payment method
	hasPaymentMethod := customer.PrimaryPaymentSourceId != "" || customer.PaymentMethod != nil

	return hasPaymentMethod, nil
}

// RetrieveInvoice retrieves an invoice from Chargebee
func (s *InvoiceService) RetrieveInvoice(ctx context.Context, invoiceID string) (*InvoiceResponse, error) {
	s.logger.Infow("retrieving invoice from Chargebee",
		"invoice_id", invoiceID)

	// Retrieve invoice using client wrapper
	result, err := s.client.RetrieveInvoice(ctx, invoiceID, &chargebeeInvoice.RetrieveRequestParams{})
	if err != nil {
		s.logger.Errorw("failed to retrieve invoice from Chargebee",
			"invoice_id", invoiceID,
			"error", err)
		return nil, ierr.NewError("failed to retrieve invoice from Chargebee").
			WithReportableDetails(map[string]interface{}{
				"error":      err.Error(),
				"invoice_id": invoiceID,
			}).
			WithHint("Check if invoice exists in Chargebee").
			Mark(ierr.ErrNotFound)
	}

	invoiceData := result.Invoice

	s.logger.Infow("successfully retrieved invoice from Chargebee",
		"invoice_id", invoiceData.Id,
		"customer_id", invoiceData.CustomerId)

	// Convert to our DTO format
	invoiceResponse := &InvoiceResponse{
		ID:              invoiceData.Id,
		CustomerID:      invoiceData.CustomerId,
		Status:          string(invoiceData.Status),
		AutoCollection:  "", // Not available in invoice object
		Total:           invoiceData.Total,
		AmountDue:       invoiceData.AmountDue,
		AmountPaid:      invoiceData.AmountPaid,
		CurrencyCode:    invoiceData.CurrencyCode,
		Date:            time.Unix(invoiceData.Date, 0),
		CreatedAt:       time.Unix(invoiceData.GeneratedAt, 0), // Use GeneratedAt as CreatedAt
		UpdatedAt:       time.Unix(invoiceData.UpdatedAt, 0),
		ResourceVersion: invoiceData.ResourceVersion,
	}

	if invoiceData.DueDate > 0 {
		dueDate := time.Unix(invoiceData.DueDate, 0)
		invoiceResponse.DueDate = &dueDate
	}

	// Convert line items if present
	if len(invoiceData.LineItems) > 0 {
		lineItems := make([]InvoiceLineItemResponse, 0, len(invoiceData.LineItems))
		for _, item := range invoiceData.LineItems {
			lineItem := InvoiceLineItemResponse{
				ID:          item.Id,
				ItemPriceID: item.EntityId, // EntityId contains the item price ID
				EntityType:  string(item.EntityType),
				Quantity:    int(item.Quantity),
				UnitAmount:  item.UnitAmount,
				Amount:      item.Amount,
				Description: item.Description,
				DateFrom:    time.Unix(item.DateFrom, 0),
				DateTo:      time.Unix(item.DateTo, 0),
			}
			lineItems = append(lineItems, lineItem)
		}
		invoiceResponse.LineItems = lineItems
	}

	return invoiceResponse, nil
}

// ReconcileInvoicePayment reconciles an invoice payment when a payment succeeds in Chargebee
// This is called from the webhook handler after a payment_succeeded event
func (s *InvoiceService) ReconcileInvoicePayment(ctx context.Context, invoiceID string, paymentAmount decimal.Decimal, invoiceService interfaces.InvoiceService) error {
	s.logger.Infow("starting payment reconciliation with invoice",
		"invoice_id", invoiceID,
		"payment_amount", paymentAmount.String())

	// Get the invoice
	invoiceResp, err := invoiceService.GetInvoice(ctx, invoiceID)
	if err != nil {
		s.logger.Errorw("failed to get invoice for reconciliation",
			"error", err,
			"invoice_id", invoiceID)
		return err
	}

	// Calculate new amounts
	newAmountPaid := invoiceResp.AmountPaid.Add(paymentAmount)
	newAmountRemaining := invoiceResp.AmountDue.Sub(newAmountPaid)

	// Determine payment status
	var newPaymentStatus types.PaymentStatus
	if newAmountRemaining.IsZero() {
		newPaymentStatus = types.PaymentStatusSucceeded
	} else if newAmountRemaining.IsNegative() {
		newPaymentStatus = types.PaymentStatusOverpaid
		newAmountRemaining = decimal.Zero
	} else {
		newPaymentStatus = types.PaymentStatusPending
	}

	s.logger.Infow("calculated new amounts for reconciliation",
		"invoice_id", invoiceID,
		"payment_amount", paymentAmount.String(),
		"new_amount_paid", newAmountPaid.String(),
		"new_amount_remaining", newAmountRemaining.String(),
		"new_payment_status", newPaymentStatus)

	// Update invoice
	err = invoiceService.ReconcilePaymentStatus(ctx, invoiceID, newPaymentStatus, &paymentAmount)
	if err != nil {
		s.logger.Errorw("failed to update invoice payment status",
			"error", err,
			"invoice_id", invoiceID)
		return err
	}

	s.logger.Infow("successfully reconciled invoice",
		"invoice_id", invoiceID,
		"payment_amount", paymentAmount.String(),
		"new_payment_status", newPaymentStatus)

	return nil
}

// GetFlexPriceInvoiceIDByChargebeeInvoiceID retrieves the FlexPrice invoice ID from entity mapping
func (s *InvoiceService) GetFlexPriceInvoiceIDByChargebeeInvoiceID(ctx context.Context, chargebeeInvoiceID string) (string, error) {
	filter := types.NewEntityIntegrationMappingFilter()
	filter.ProviderEntityIDs = []string{chargebeeInvoiceID}
	filter.ProviderTypes = []string{string(types.SecretProviderChargebee)}
	filter.EntityType = types.IntegrationEntityTypeInvoice

	mappings, err := s.entityIntegrationMappingRepo.List(ctx, filter)
	if err != nil {
		return "", ierr.WithError(err).
			WithHint("Failed to get FlexPrice invoice mapping").
			Mark(ierr.ErrDatabase)
	}

	if len(mappings) == 0 {
		return "", ierr.NewError("FlexPrice invoice not found for Chargebee invoice").
			WithHint("Chargebee invoice not synced to FlexPrice").
			WithReportableDetails(map[string]interface{}{
				"chargebee_invoice_id": chargebeeInvoiceID,
			}).
			Mark(ierr.ErrNotFound)
	}

	return mappings[0].EntityID, nil
}

// ProcessChargebeePaymentFromWebhook processes a Chargebee payment and creates a FlexPrice payment record
func (s *InvoiceService) ProcessChargebeePaymentFromWebhook(
	ctx context.Context,
	flexpriceInvoiceID string,
	chargebeeTransactionID string,
	chargebeeInvoiceID string,
	amount decimal.Decimal,
	currency string,
	paymentMethod string,
	invoiceService interfaces.InvoiceService,
	paymentService interfaces.PaymentService,
) error {
	s.logger.Infow("processing Chargebee payment from webhook",
		"flexprice_invoice_id", flexpriceInvoiceID,
		"chargebee_invoice_id", chargebeeInvoiceID,
		"chargebee_transaction_id", chargebeeTransactionID,
		"amount", amount.String(),
		"currency", currency)

	// Step 1: Check if payment already exists (idempotency check)
	// This prevents duplicate payment records when Chargebee retries webhooks
	exists, err := s.PaymentExistsByGatewayPaymentID(ctx, chargebeeTransactionID, paymentService)
	if err != nil {
		s.logger.Errorw("failed to check if payment exists by gateway payment ID",
			"error", err,
			"chargebee_transaction_id", chargebeeTransactionID)
		// Continue processing on error - fail-safe behavior
	} else if exists {
		s.logger.Infow("payment already exists for this Chargebee transaction, skipping",
			"chargebee_transaction_id", chargebeeTransactionID,
			"chargebee_invoice_id", chargebeeInvoiceID,
			"flexprice_invoice_id", flexpriceInvoiceID)
		return nil
	}

	// Step 2: Get FlexPrice invoice
	invoiceResp, err := invoiceService.GetInvoice(ctx, flexpriceInvoiceID)
	if err != nil {
		s.logger.Errorw("failed to get FlexPrice invoice",
			"error", err,
			"flexprice_invoice_id", flexpriceInvoiceID)
		return ierr.WithError(err).
			WithHint("Failed to get FlexPrice invoice").
			Mark(ierr.ErrDatabase)
	}

	// Step 3: Check if invoice is already succeeded (secondary check)
	if invoiceResp.PaymentStatus == types.PaymentStatusSucceeded {
		s.logger.Infow("invoice already succeeded, skipping duplicate payment",
			"flexprice_invoice_id", flexpriceInvoiceID,
			"chargebee_invoice_id", chargebeeInvoiceID)
		return nil
	}

	// Step 3: Create payment record in FlexPrice
	now := time.Now()
	createPaymentReq := dto.CreatePaymentRequest{
		Amount:            amount,
		Currency:          currency,
		PaymentMethodType: types.PaymentMethodTypeCard, // Default to card
		DestinationType:   types.PaymentDestinationTypeInvoice,
		DestinationID:     flexpriceInvoiceID,
		ProcessPayment:    false, // Already processed by Chargebee
		Metadata: types.Metadata{
			"chargebee_transaction_id": chargebeeTransactionID,
			"chargebee_invoice_id":     chargebeeInvoiceID,
			"payment_method":           paymentMethod,
			"source":                   "chargebee_webhook",
		},
	}

	payment, err := paymentService.CreatePayment(ctx, &createPaymentReq)
	if err != nil {
		s.logger.Errorw("failed to create payment record",
			"error", err,
			"flexprice_invoice_id", flexpriceInvoiceID,
			"chargebee_transaction_id", chargebeeTransactionID)
		return ierr.WithError(err).
			WithHint("Failed to create payment record").
			Mark(ierr.ErrDatabase)
	}

	s.logger.Infow("created payment record",
		"payment_id", payment.ID,
		"flexprice_invoice_id", flexpriceInvoiceID,
		"chargebee_transaction_id", chargebeeTransactionID,
		"amount", amount.String())

	// Update payment to succeeded status
	paymentStatus := string(types.PaymentStatusSucceeded)
	updatePaymentReq := dto.UpdatePaymentRequest{
		PaymentStatus:    &paymentStatus,
		GatewayPaymentID: &chargebeeTransactionID,
		SucceededAt:      &now,
	}

	_, err = paymentService.UpdatePayment(ctx, payment.ID, updatePaymentReq)
	if err != nil {
		s.logger.Errorw("failed to update payment to succeeded",
			"error", err,
			"payment_id", payment.ID,
			"chargebee_transaction_id", chargebeeTransactionID)
		return ierr.WithError(err).
			WithHint("Failed to update payment status").
			Mark(ierr.ErrDatabase)
	}

	s.logger.Infow("updated payment to succeeded",
		"payment_id", payment.ID,
		"chargebee_transaction_id", chargebeeTransactionID,
		"amount", amount.String())

	// Step 4: Reconcile invoice
	err = s.ReconcileInvoicePayment(ctx, flexpriceInvoiceID, amount, invoiceService)
	if err != nil {
		s.logger.Errorw("failed to reconcile payment with invoice",
			"error", err,
			"flexprice_invoice_id", flexpriceInvoiceID,
			"amount", amount.String())
		return ierr.WithError(err).
			WithHint("Failed to reconcile invoice payment").
			Mark(ierr.ErrDatabase)
	}

	s.logger.Infow("successfully processed Chargebee payment",
		"payment_id", payment.ID,
		"flexprice_invoice_id", flexpriceInvoiceID,
		"chargebee_transaction_id", chargebeeTransactionID)

	return nil
}

// PaymentExistsByGatewayPaymentID checks if a payment already exists with the given gateway payment ID
// This is used for idempotency checks to prevent duplicate payment records from webhook retries
func (s *InvoiceService) PaymentExistsByGatewayPaymentID(
	ctx context.Context,
	gatewayPaymentID string,
	paymentService interfaces.PaymentService,
) (bool, error) {
	if gatewayPaymentID == "" {
		return false, nil
	}

	// Create filter to query payments by gateway_payment_id
	filter := types.NewNoLimitPaymentFilter()
	limit := 1
	filter.QueryFilter.Limit = &limit
	filter.GatewayPaymentID = &gatewayPaymentID

	// Query payments
	payments, err := paymentService.ListPayments(ctx, filter)
	if err != nil {
		return false, err
	}

	// Return true if any payment exists with this gateway payment ID
	return len(payments.Items) > 0, nil
}
