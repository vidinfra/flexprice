package quickbooks

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/entityintegrationmapping"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/interfaces"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// QuickBooksPaymentService defines the interface for QuickBooks payment operations
type QuickBooksPaymentService interface {
	// SyncPaymentToQuickBooks syncs a Flexprice payment to QuickBooks
	SyncPaymentToQuickBooks(ctx context.Context, paymentID string, paymentService interfaces.PaymentService, invoiceService interfaces.InvoiceService) error

	// HandleExternalPaymentFromWebhook processes a QuickBooks payment webhook
	HandleExternalPaymentFromWebhook(ctx context.Context, qbPaymentID string, paymentService interfaces.PaymentService, invoiceService interfaces.InvoiceService) error
}

// PaymentService handles QuickBooks payment operations
type PaymentService struct {
	client                       QuickBooksClient
	invoiceRepo                  invoice.Repository
	entityIntegrationMappingRepo entityintegrationmapping.Repository
	logger                       *logger.Logger
}

// PaymentServiceParams contains parameters for creating a PaymentService
type PaymentServiceParams struct {
	Client                       QuickBooksClient
	InvoiceRepo                  invoice.Repository
	EntityIntegrationMappingRepo entityintegrationmapping.Repository
	Logger                       *logger.Logger
}

// NewPaymentService creates a new QuickBooks payment service
func NewPaymentService(params PaymentServiceParams) QuickBooksPaymentService {
	return &PaymentService{
		client:                       params.Client,
		invoiceRepo:                  params.InvoiceRepo,
		entityIntegrationMappingRepo: params.EntityIntegrationMappingRepo,
		logger:                       params.Logger,
	}
}

// SyncPaymentToQuickBooks syncs a Flexprice payment to QuickBooks (OUTBOUND SYNC)
//
// FLOW: Flexprice → QuickBooks
// TRIGGER: When an invoice is paid in Flexprice (via Stripe, Razorpay, etc.)
//
// WHAT IT DOES:
// 1. Gets the payment details from Flexprice
// 2. Finds the linked QuickBooks invoice via entity mapping
// 3. Creates a Payment entity in QuickBooks
// 4. QuickBooks automatically marks the invoice as PAID and CLOSED
// 5. Stores the payment mapping (Flexprice payment ID ↔ QuickBooks payment ID)
// 6. Updates Flexprice payment metadata with QuickBooks details
//
// IMPORTANT: Creating a Payment in QuickBooks automatically closes the invoice!
func (s *PaymentService) SyncPaymentToQuickBooks(ctx context.Context, paymentID string, paymentService interfaces.PaymentService, invoiceService interfaces.InvoiceService) error {
	s.logger.Debugw("syncing payment to QuickBooks",
		"payment_id", paymentID)

	// Get the payment
	paymentResp, err := paymentService.GetPayment(ctx, paymentID)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to get payment for QuickBooks sync").
			Mark(ierr.ErrNotFound)
	}

	// Only sync invoice payments
	if paymentResp.DestinationType != types.PaymentDestinationTypeInvoice {
		s.logger.Debugw("skipping non-invoice payment for QuickBooks sync",
			"payment_id", paymentID,
			"destination_type", paymentResp.DestinationType)
		return nil
	}

	// Check if payment is already synced
	existingMapping, err := s.findMappingByEntityAndProvider(ctx, paymentID, types.IntegrationEntityTypePayment)
	if err != nil && !ierr.IsNotFound(err) {
		return ierr.WithError(err).
			WithHint("Failed to check existing payment mapping").
			Mark(ierr.ErrDatabase)
	}
	if existingMapping != nil {
		s.logger.Debugw("payment already synced to QuickBooks",
			"payment_id", paymentID,
			"quickbooks_payment_id", existingMapping.ProviderEntityID)
		return nil
	}

	// Find QuickBooks invoice mapping
	invoiceMapping, err := s.findMappingByEntityAndProvider(ctx, paymentResp.DestinationID, types.IntegrationEntityTypeInvoice)
	if err != nil {
		if ierr.IsNotFound(err) {
			s.logger.Debugw("invoice not synced to QuickBooks, skipping payment sync",
				"payment_id", paymentID,
				"invoice_id", paymentResp.DestinationID)
			return nil
		}
		return ierr.WithError(err).
			WithHint("Failed to get invoice mapping for QuickBooks").
			Mark(ierr.ErrDatabase)
	}

	qbInvoiceID := invoiceMapping.ProviderEntityID

	// Get the QuickBooks invoice to get customer reference
	qbInvoice, err := s.client.GetInvoice(ctx, qbInvoiceID)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to get QuickBooks invoice for payment").
			Mark(ierr.ErrHTTPClient)
	}

	// Create payment in QuickBooks
	paymentAmount, _ := paymentResp.Amount.Float64()
	txnDate := time.Now().Format("2006-01-02")
	if paymentResp.SucceededAt != nil {
		txnDate = paymentResp.SucceededAt.Format("2006-01-02")
	}

	createReq := &PaymentCreateRequest{
		CustomerRef: AccountRef{
			Value: qbInvoice.CustomerRef.Value,
		},
		TotalAmt: paymentAmount,
		TxnDate:  txnDate,
		Line: []PaymentLine{
			{
				Amount: paymentAmount,
				LinkedTxn: []LinkedTxn{
					{
						TxnId:   qbInvoiceID,
						TxnType: "Invoice",
					},
				},
			},
		},
		PrivateNote: fmt.Sprintf("Payment recorded by: flexprice\nFlexprice Invoice ID: %s\nPayment Method: %s", paymentResp.DestinationID, paymentResp.PaymentMethodType),
	}

	qbPayment, err := s.client.CreatePayment(ctx, createReq)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to create payment in QuickBooks").
			Mark(ierr.ErrHTTPClient)
	}

	// Create entity integration mapping for the payment
	mapping := &entityintegrationmapping.EntityIntegrationMapping{
		ID:               types.GenerateUUIDWithPrefix(types.UUID_PREFIX_ENTITY_INTEGRATION_MAPPING),
		EntityID:         paymentID,
		EntityType:       types.IntegrationEntityTypePayment,
		ProviderType:     string(types.SecretProviderQuickBooks),
		ProviderEntityID: qbPayment.ID,
		Metadata: map[string]interface{}{
			"quickbooks_invoice_id": qbInvoiceID,
			"flexprice_invoice_id":  paymentResp.DestinationID,
			"payment_amount":        paymentResp.Amount.String(),
			"payment_method":        string(paymentResp.PaymentMethodType),
			"synced_at":             time.Now().UTC().Format(time.RFC3339),
			"sync_source":           "flexprice",
		},
		EnvironmentID: types.GetEnvironmentID(ctx),
		BaseModel:     types.GetDefaultBaseModel(ctx),
	}

	if err := s.entityIntegrationMappingRepo.Create(ctx, mapping); err != nil {
		s.logger.Errorw("failed to create payment mapping, payment was created in QuickBooks",
			"error", err,
			"payment_id", paymentID,
			"quickbooks_payment_id", qbPayment.ID)
		// Don't return error since payment was created successfully
	}

	// Update payment metadata with QuickBooks information
	currentMetadata := paymentResp.Metadata
	if currentMetadata == nil {
		currentMetadata = make(map[string]string)
	}

	// Store QuickBooks sync information in payment metadata
	currentMetadata["quickbooks_payment_id"] = qbPayment.ID
	currentMetadata["quickbooks_invoice_id"] = qbInvoiceID
	currentMetadata["entity_mapping_id"] = mapping.ID
	currentMetadata["quickbooks_sync_source"] = "flexprice_outbound"

	// Update payment with metadata
	updateReq := &dto.UpdatePaymentRequest{
		Metadata: &currentMetadata,
	}

	if _, err := paymentService.UpdatePayment(ctx, paymentID, *updateReq); err != nil {
		s.logger.Errorw("failed to update payment metadata with QuickBooks info",
			"error", err,
			"payment_id", paymentID,
			"quickbooks_payment_id", qbPayment.ID)
		// Don't return error since payment was created successfully in QuickBooks
	}

	s.logger.Infow("successfully synced payment to QuickBooks",
		"payment_id", paymentID,
		"quickbooks_payment_id", qbPayment.ID,
		"quickbooks_invoice_id", qbInvoiceID,
		"amount", paymentAmount)

	return nil
}

// HandleExternalPaymentFromWebhook processes a QuickBooks payment webhook (INBOUND SYNC)
//
// FLOW: QuickBooks → Flexprice
// TRIGGER: When a payment is recorded in QuickBooks (via webhook notification)
//
// WHAT IT DOES:
// 1. Receives webhook notification (minimal data: just payment ID)
// 2. Calls QuickBooks API to get full payment details (amount, linked invoice, date)
// 3. Finds the Flexprice invoice via entity mapping
// 4. Marks the Flexprice invoice as SUCCEEDED (no payment record created!)
// 5. Updates invoice metadata with QuickBooks payment details
//
// IMPORTANT: We DON'T create a payment record in Flexprice, just mark invoice as paid!
// WHY: Simpler, avoids precision issues, invoice has all needed info (amount, date, status)
func (s *PaymentService) HandleExternalPaymentFromWebhook(ctx context.Context, qbPaymentID string, paymentService interfaces.PaymentService, invoiceService interfaces.InvoiceService) error {
	s.logger.Infow("processing QuickBooks payment webhook - simple approach",
		"quickbooks_payment_id", qbPaymentID)

	// Fetch payment details from QuickBooks to get the linked invoice
	qbPayment, err := s.client.GetPayment(ctx, qbPaymentID)
	if err != nil {
		s.logger.Errorw("failed to get payment from QuickBooks",
			"error", err,
			"quickbooks_payment_id", qbPaymentID)
		return nil // Don't fail webhook processing
	}

	// Find invoice references in payment lines
	var qbInvoiceID string
	for _, line := range qbPayment.Line {
		for _, txn := range line.LinkedTxn {
			if txn.TxnType == "Invoice" {
				qbInvoiceID = txn.TxnId
				break
			}
		}
		if qbInvoiceID != "" {
			break
		}
	}

	if qbInvoiceID == "" {
		s.logger.Debugw("no invoice linked to QuickBooks payment, skipping",
			"quickbooks_payment_id", qbPaymentID)
		return nil
	}

	// Find Flexprice invoice via mapping
	invoiceMapping, err := s.findInvoiceMappingByProviderID(ctx, qbInvoiceID)
	if err != nil {
		if ierr.IsNotFound(err) {
			s.logger.Debugw("invoice not found in Flexprice, skipping payment sync",
				"quickbooks_payment_id", qbPaymentID,
				"quickbooks_invoice_id", qbInvoiceID)
			return nil
		}
		s.logger.Errorw("failed to find invoice mapping",
			"error", err,
			"quickbooks_invoice_id", qbInvoiceID)
		return nil
	}

	flexpriceInvoiceID := invoiceMapping.EntityID

	// Get the invoice
	invoice, err := s.invoiceRepo.Get(ctx, flexpriceInvoiceID)
	if err != nil {
		s.logger.Errorw("failed to get Flexprice invoice",
			"error", err,
			"invoice_id", flexpriceInvoiceID)
		return nil
	}

	// Check if already succeeded
	if invoice.PaymentStatus == types.PaymentStatusSucceeded {
		s.logger.Infow("invoice already succeeded, skipping",
			"invoice_id", flexpriceInvoiceID,
			"quickbooks_payment_id", qbPaymentID)
		return nil
	}

	// Update invoice to succeeded with offline payment method
	now := time.Now().UTC()
	invoice.PaymentStatus = types.PaymentStatusSucceeded
	invoice.PaidAt = &now
	invoice.AmountPaid = invoice.AmountDue
	invoice.AmountRemaining = decimal.Zero

	// Add QuickBooks sync details to invoice metadata
	if invoice.Metadata == nil {
		invoice.Metadata = make(types.Metadata)
	}
	invoice.Metadata["payment_recorded_by"] = "quickbooks"
	invoice.Metadata["payment_method"] = "offline"
	invoice.Metadata["quickbooks_payment_id"] = qbPaymentID
	invoice.Metadata["quickbooks_invoice_id"] = qbInvoiceID
	invoice.Metadata["entity_mapping_id"] = invoiceMapping.ID
	invoice.Metadata["payment_synced_at"] = now.Format(time.RFC3339)

	if err := s.invoiceRepo.Update(ctx, invoice); err != nil {
		s.logger.Errorw("failed to update invoice status",
			"error", err,
			"invoice_id", flexpriceInvoiceID)
		return nil
	}

	s.logger.Infow("successfully marked Flexprice invoice as paid from QuickBooks",
		"quickbooks_payment_id", qbPaymentID,
		"quickbooks_invoice_id", qbInvoiceID,
		"flexprice_invoice_id", flexpriceInvoiceID)

	return nil
}

// findMappingByEntityAndProvider finds a mapping by entity ID, entity type, and provider
func (s *PaymentService) findMappingByEntityAndProvider(ctx context.Context, entityID string, entityType types.IntegrationEntityType) (*entityintegrationmapping.EntityIntegrationMapping, error) {
	filter := &types.EntityIntegrationMappingFilter{
		EntityID:      entityID,
		EntityType:    entityType,
		ProviderTypes: []string{string(types.SecretProviderQuickBooks)},
		QueryFilter:   types.NewDefaultQueryFilter(),
	}

	mappings, err := s.entityIntegrationMappingRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	if len(mappings) == 0 {
		return nil, ierr.NewError("mapping not found").Mark(ierr.ErrNotFound)
	}

	return mappings[0], nil
}

// findInvoiceMappingByProviderID finds an invoice mapping by QuickBooks invoice ID
func (s *PaymentService) findInvoiceMappingByProviderID(ctx context.Context, qbInvoiceID string) (*entityintegrationmapping.EntityIntegrationMapping, error) {
	filter := &types.EntityIntegrationMappingFilter{
		ProviderTypes:     []string{string(types.SecretProviderQuickBooks)},
		ProviderEntityIDs: []string{qbInvoiceID},
		EntityType:        types.IntegrationEntityTypeInvoice,
		QueryFilter:       types.NewDefaultQueryFilter(),
	}

	mappings, err := s.entityIntegrationMappingRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	if len(mappings) == 0 {
		return nil, ierr.NewError("invoice mapping not found").Mark(ierr.ErrNotFound)
	}

	return mappings[0], nil
}
