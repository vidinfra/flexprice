package webhook

import (
	"context"
	"encoding/json"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/entityintegrationmapping"
	"github.com/flexprice/flexprice/internal/integration/chargebee"
	"github.com/flexprice/flexprice/internal/interfaces"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// Handler handles Chargebee webhook events
type Handler struct {
	client                       chargebee.ChargebeeClient
	invoiceSvc                   *chargebee.InvoiceService
	entityIntegrationMappingRepo entityintegrationmapping.Repository
	logger                       *logger.Logger
}

// NewHandler creates a new Chargebee webhook handler
func NewHandler(
	client chargebee.ChargebeeClient,
	invoiceSvc *chargebee.InvoiceService,
	entityIntegrationMappingRepo entityintegrationmapping.Repository,
	logger *logger.Logger,
) *Handler {
	return &Handler{
		client:                       client,
		invoiceSvc:                   invoiceSvc,
		entityIntegrationMappingRepo: entityIntegrationMappingRepo,
		logger:                       logger,
	}
}

// ServiceDependencies contains all service dependencies needed by webhook handlers
type ServiceDependencies = interfaces.ServiceDependencies

// HandleWebhookEvent processes a Chargebee webhook event
// This function never returns errors to ensure webhooks always return 200 OK
// All errors are logged internally to prevent Chargebee from retrying
func (h *Handler) HandleWebhookEvent(ctx context.Context, event *ChargebeeWebhookEvent, environmentID string, services *ServiceDependencies) error {
	h.logger.Infow("processing Chargebee webhook event",
		"event_type", event.EventType,
		"event_id", event.ID,
		"environment_id", environmentID,
		"occurred_at", event.OccurredAt,
	)

	eventType := ChargebeeEventType(event.EventType)

	switch eventType {
	case EventPaymentSucceeded:
		return h.handlePaymentSucceeded(ctx, event, environmentID, services)
	case EventPaymentFailed:
		return h.handlePaymentFailed(ctx, event, environmentID, services)
	default:
		h.logger.Infow("unhandled Chargebee webhook event type", "type", event.EventType)
		return nil // Not an error, just unhandled
	}
}

// handlePaymentSucceeded handles payment_succeeded webhook
func (h *Handler) handlePaymentSucceeded(ctx context.Context, event *ChargebeeWebhookEvent, environmentID string, services *ServiceDependencies) error {
	// Parse the content
	var content ChargebeeWebhookContent
	if err := json.Unmarshal(event.Content, &content); err != nil {
		h.logger.Errorw("failed to parse webhook content",
			"error", err,
			"event_id", event.ID)
		return nil // Don't return error - webhook should always succeed
	}

	if content.Transaction == nil {
		h.logger.Warnw("no transaction found in payment_succeeded event",
			"event_id", event.ID)
		return nil
	}

	if content.Invoice == nil {
		h.logger.Warnw("no invoice found in payment_succeeded event",
			"event_id", event.ID)
		return nil
	}

	transaction := content.Transaction
	invoice := content.Invoice

	h.logger.Infow("received payment_succeeded webhook",
		"chargebee_transaction_id", transaction.ID,
		"chargebee_invoice_id", invoice.ID,
		"amount", transaction.Amount,
		"currency", transaction.CurrencyCode,
		"status", transaction.Status,
		"environment_id", environmentID,
	)

	// Get FlexPrice invoice ID from entity mapping
	filter := types.NewNoLimitEntityIntegrationMappingFilter()
	filter.ProviderEntityIDs = []string{invoice.ID}
	filter.ProviderTypes = []string{string(types.SecretProviderChargebee)}
	filter.EntityType = types.IntegrationEntityTypeInvoice

	mappings, err := h.entityIntegrationMappingRepo.List(ctx, filter)
	if err != nil {
		h.logger.Errorw("failed to get invoice mapping",
			"error", err,
			"chargebee_invoice_id", invoice.ID)
		return nil // Don't fail webhook processing
	}

	if len(mappings) == 0 {
		h.logger.Warnw("no FlexPrice invoice found for Chargebee invoice",
			"chargebee_invoice_id", invoice.ID,
			"event_id", event.ID)
		return nil // Not a FlexPrice-initiated invoice
	}

	flexpriceInvoiceID := mappings[0].EntityID

	h.logger.Infow("found FlexPrice invoice for payment",
		"flexprice_invoice_id", flexpriceInvoiceID,
		"chargebee_invoice_id", invoice.ID,
		"chargebee_transaction_id", transaction.ID)

	// Get the invoice to check current status
	invoiceResp, err := services.InvoiceService.GetInvoice(ctx, flexpriceInvoiceID)
	if err != nil {
		h.logger.Errorw("failed to get invoice",
			"error", err,
			"flexprice_invoice_id", flexpriceInvoiceID,
			"chargebee_invoice_id", invoice.ID)
		return nil // Don't fail webhook processing
	}

	if invoiceResp == nil {
		h.logger.Warnw("invoice not found",
			"flexprice_invoice_id", flexpriceInvoiceID,
			"chargebee_invoice_id", invoice.ID)
		return nil
	}

	// Check if invoice is already paid
	if invoiceResp.PaymentStatus == types.PaymentStatusSucceeded {
		h.logger.Infow("invoice already paid",
			"flexprice_invoice_id", flexpriceInvoiceID,
			"chargebee_invoice_id", invoice.ID,
			"status", invoiceResp.PaymentStatus)
		return nil
	}

	// Convert amount from smallest currency unit (cents) to standard unit
	paymentAmount := decimal.NewFromInt(transaction.Amount).Div(decimal.NewFromInt(100))

	h.logger.Infow("creating payment record for Chargebee transaction",
		"flexprice_invoice_id", flexpriceInvoiceID,
		"chargebee_invoice_id", invoice.ID,
		"chargebee_transaction_id", transaction.ID,
		"payment_amount", paymentAmount.String(),
		"currency", transaction.CurrencyCode)

	// Create payment record in FlexPrice
	now := time.Now()

	createPaymentReq := dto.CreatePaymentRequest{
		Amount:            paymentAmount,
		Currency:          transaction.CurrencyCode,
		PaymentMethodType: types.PaymentMethodTypeCard, // Default to card
		DestinationType:   types.PaymentDestinationTypeInvoice,
		DestinationID:     flexpriceInvoiceID,
		ProcessPayment:    false, // Already processed by Chargebee
		Metadata: types.Metadata{
			"chargebee_transaction_id": transaction.ID,
			"chargebee_invoice_id":     invoice.ID,
			"payment_method":           transaction.PaymentMethod,
			"source":                   "chargebee_webhook",
		},
	}

	payment, err := services.PaymentService.CreatePayment(ctx, &createPaymentReq)
	if err != nil {
		h.logger.Errorw("failed to create payment record",
			"error", err,
			"flexprice_invoice_id", flexpriceInvoiceID,
			"chargebee_transaction_id", transaction.ID)
		// Continue to reconcile invoice even if payment creation fails
	} else {
		h.logger.Infow("created payment record",
			"payment_id", payment.ID,
			"flexprice_invoice_id", flexpriceInvoiceID,
			"chargebee_transaction_id", transaction.ID,
			"payment_amount", paymentAmount.String())

		// Update payment to succeeded status
		paymentStatus := string(types.PaymentStatusSucceeded)
		updatePaymentReq := dto.UpdatePaymentRequest{
			PaymentStatus:    &paymentStatus,
			GatewayPaymentID: &transaction.ID,
			SucceededAt:      &now,
		}

		_, err = services.PaymentService.UpdatePayment(ctx, payment.ID, updatePaymentReq)
		if err != nil {
			h.logger.Errorw("failed to update payment to succeeded",
				"error", err,
				"payment_id", payment.ID,
				"chargebee_transaction_id", transaction.ID)
		}
	}

	h.logger.Infow("reconciling payment with invoice",
		"flexprice_invoice_id", flexpriceInvoiceID,
		"chargebee_invoice_id", invoice.ID,
		"chargebee_transaction_id", transaction.ID,
		"payment_amount", paymentAmount.String(),
		"currency", transaction.CurrencyCode)

	// Reconcile invoice using the invoice service method
	err = h.invoiceSvc.ReconcileInvoicePayment(ctx, flexpriceInvoiceID, paymentAmount, services.InvoiceService)
	if err != nil {
		h.logger.Errorw("failed to reconcile payment with invoice",
			"error", err,
			"flexprice_invoice_id", flexpriceInvoiceID,
			"chargebee_invoice_id", invoice.ID,
			"payment_amount", paymentAmount.String())
		// Don't fail - invoice reconciliation is not critical for webhook success
	} else {
		h.logger.Infow("successfully reconciled payment with invoice",
			"flexprice_invoice_id", flexpriceInvoiceID,
			"chargebee_invoice_id", invoice.ID,
			"payment_amount", paymentAmount.String())
	}

	return nil
}

// handlePaymentFailed handles payment_failed webhook
func (h *Handler) handlePaymentFailed(ctx context.Context, event *ChargebeeWebhookEvent, environmentID string, services *ServiceDependencies) error {
	// Parse the content
	var content ChargebeeWebhookContent
	if err := json.Unmarshal(event.Content, &content); err != nil {
		h.logger.Errorw("failed to parse webhook content",
			"error", err,
			"event_id", event.ID)
		return nil // Don't return error - webhook should always succeed
	}

	if content.Transaction == nil {
		h.logger.Warnw("no transaction found in payment_failed event",
			"event_id", event.ID)
		return nil
	}

	if content.Invoice == nil {
		h.logger.Warnw("no invoice found in payment_failed event",
			"event_id", event.ID)
		return nil
	}

	transaction := content.Transaction
	invoice := content.Invoice

	h.logger.Infow("received payment_failed webhook",
		"chargebee_transaction_id", transaction.ID,
		"chargebee_invoice_id", invoice.ID,
		"amount", transaction.Amount,
		"currency", transaction.CurrencyCode,
		"status", transaction.Status,
		"environment_id", environmentID,
	)

	// Get FlexPrice invoice ID from entity mapping
	filter := types.NewNoLimitEntityIntegrationMappingFilter()
	filter.ProviderEntityIDs = []string{invoice.ID}
	filter.ProviderTypes = []string{string(types.SecretProviderChargebee)}
	filter.EntityType = types.IntegrationEntityTypeInvoice

	mappings, err := h.entityIntegrationMappingRepo.List(ctx, filter)
	if err != nil {
		h.logger.Errorw("failed to get invoice mapping",
			"error", err,
			"chargebee_invoice_id", invoice.ID)
		return nil // Don't fail webhook processing
	}

	if len(mappings) == 0 {
		h.logger.Warnw("no FlexPrice invoice found for Chargebee invoice",
			"chargebee_invoice_id", invoice.ID,
			"event_id", event.ID)
		return nil // Not a FlexPrice-initiated invoice
	}

	flexpriceInvoiceID := mappings[0].EntityID

	h.logger.Infow("processing payment failure for FlexPrice invoice",
		"flexprice_invoice_id", flexpriceInvoiceID,
		"chargebee_invoice_id", invoice.ID,
		"chargebee_transaction_id", transaction.ID)

	// Get the invoice to check current status
	invoiceResp, err := services.InvoiceService.GetInvoice(ctx, flexpriceInvoiceID)
	if err != nil {
		h.logger.Errorw("failed to get invoice",
			"error", err,
			"flexprice_invoice_id", flexpriceInvoiceID,
			"chargebee_invoice_id", invoice.ID)
		return nil // Don't fail webhook processing
	}

	if invoiceResp == nil {
		h.logger.Warnw("invoice not found",
			"flexprice_invoice_id", flexpriceInvoiceID,
			"chargebee_invoice_id", invoice.ID)
		return nil
	}

	// Check if invoice is already succeeded
	if invoiceResp.PaymentStatus == types.PaymentStatusSucceeded {
		h.logger.Infow("ignoring payment.failed webhook for succeeded invoice",
			"flexprice_invoice_id", flexpriceInvoiceID,
			"chargebee_invoice_id", invoice.ID)
		return nil
	}

	// For payment failures, we just log it
	// We don't change the invoice status as the customer might retry payment
	h.logger.Infow("payment failed for invoice",
		"flexprice_invoice_id", flexpriceInvoiceID,
		"chargebee_invoice_id", invoice.ID,
		"chargebee_transaction_id", transaction.ID)

	return nil
}
