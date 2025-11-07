package webhook

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/entityintegrationmapping"
	"github.com/flexprice/flexprice/internal/integration/razorpay"
	"github.com/flexprice/flexprice/internal/interfaces"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// Handler handles Razorpay webhook events
type Handler struct {
	client                       razorpay.RazorpayClient
	entityIntegrationMappingRepo entityintegrationmapping.Repository
	logger                       *logger.Logger
}

// NewHandler creates a new Razorpay webhook handler
func NewHandler(
	client razorpay.RazorpayClient,
	entityIntegrationMappingRepo entityintegrationmapping.Repository,
	logger *logger.Logger,
) *Handler {
	return &Handler{
		client:                       client,
		entityIntegrationMappingRepo: entityIntegrationMappingRepo,
		logger:                       logger,
	}
}

// ServiceDependencies contains all service dependencies needed by webhook handlers
type ServiceDependencies = interfaces.ServiceDependencies

// HandleWebhookEvent processes a Razorpay webhook event
// This function never returns errors to ensure webhooks always return 200 OK
// All errors are logged internally to prevent Razorpay from retrying
func (h *Handler) HandleWebhookEvent(ctx context.Context, event *RazorpayWebhookEvent, environmentID string, services *ServiceDependencies) error {
	h.logger.Infow("processing Razorpay webhook event",
		"event_type", event.Event,
		"account_id", event.AccountID,
		"environment_id", environmentID,
		"created_at", event.CreatedAt,
	)

	eventType := RazorpayEventType(event.Event)

	switch eventType {
	case EventPaymentCaptured:
		return h.handlePaymentCaptured(ctx, event, environmentID, services)
	case EventPaymentFailed:
		return h.handlePaymentFailed(ctx, event, environmentID, services)
	default:
		h.logger.Infow("unhandled Razorpay webhook event type", "type", event.Event)
		return nil // Not an error, just unhandled
	}
}

// handlePaymentCaptured handles payment.captured webhook
func (h *Handler) handlePaymentCaptured(ctx context.Context, event *RazorpayWebhookEvent, environmentID string, services *ServiceDependencies) error {
	payment := event.Payload.Payment.Entity

	h.logger.Infow("received payment.captured webhook",
		"razorpay_payment_id", payment.ID,
		"amount", payment.Amount,
		"currency", payment.Currency,
		"status", payment.Status,
		"environment_id", environmentID,
	)

	// Get FlexPrice payment ID from notes
	flexpricePaymentID, ok := payment.Notes["flexprice_payment_id"].(string)
	if !ok || flexpricePaymentID == "" {
		h.logger.Warnw("no flexprice_payment_id found in payment notes",
			"razorpay_payment_id", payment.ID,
			"notes", payment.Notes)
		return nil // Not a FlexPrice-initiated payment
	}

	h.logger.Infow("processing FlexPrice payment capture",
		"razorpay_payment_id", payment.ID,
		"flexprice_payment_id", flexpricePaymentID)

	// Get payment record
	paymentRecord, err := services.PaymentService.GetPayment(ctx, flexpricePaymentID)
	if err != nil {
		h.logger.Errorw("failed to get payment record",
			"error", err,
			"flexprice_payment_id", flexpricePaymentID,
			"razorpay_payment_id", payment.ID)
		return nil // Don't fail webhook processing
	}

	if paymentRecord == nil {
		h.logger.Warnw("no payment record found",
			"flexprice_payment_id", flexpricePaymentID,
			"razorpay_payment_id", payment.ID)
		return nil
	}

	// Check if payment is already processed
	if paymentRecord.PaymentStatus == types.PaymentStatusSucceeded {
		h.logger.Infow("payment already processed",
			"flexprice_payment_id", flexpricePaymentID,
			"razorpay_payment_id", payment.ID,
			"status", paymentRecord.PaymentStatus)
		return nil
	}

	// Update payment status to succeeded
	paymentStatus := string(types.PaymentStatusSucceeded)
	now := time.Now()

	// Convert amount from smallest currency unit (paise) to standard unit
	amount := decimal.NewFromInt(payment.Amount).Div(decimal.NewFromInt(100))

	// Prepare gateway metadata (all values must be strings)
	gatewayMetadata := types.Metadata{
		"razorpay_payment_id": payment.ID,
		"status":              payment.Status,
		"method":              payment.Method,
		"captured":            fmt.Sprintf("%v", payment.Captured),
		"amount_paise":        fmt.Sprintf("%d", payment.Amount),
		"currency":            payment.Currency,
		"fee":                 fmt.Sprintf("%d", payment.Fee),
		"tax":                 fmt.Sprintf("%d", payment.Tax),
	}

	updateReq := dto.UpdatePaymentRequest{
		PaymentStatus: &paymentStatus,
		SucceededAt:   &now,
		Metadata:      &gatewayMetadata,
	}

	_, err = services.PaymentService.UpdatePayment(ctx, flexpricePaymentID, updateReq)
	if err != nil {
		h.logger.Errorw("failed to update payment",
			"error", err,
			"flexprice_payment_id", flexpricePaymentID,
			"razorpay_payment_id", payment.ID)
		return nil // Don't return error - webhook should always succeed
	}

	h.logger.Infow("updated payment to succeeded",
		"flexprice_payment_id", flexpricePaymentID,
		"razorpay_payment_id", payment.ID,
		"amount", amount.String(),
		"currency", payment.Currency)

	// Reconcile payment with invoice (update invoice payment status and amounts)
	h.logger.Infow("reconciling payment with invoice",
		"flexprice_payment_id", flexpricePaymentID,
		"invoice_id", paymentRecord.DestinationID,
		"payment_amount", amount.String())

	err = h.reconcilePaymentWithInvoice(ctx, flexpricePaymentID, amount, services.PaymentService, services.InvoiceService)
	if err != nil {
		h.logger.Errorw("failed to reconcile payment with invoice",
			"error", err,
			"flexprice_payment_id", flexpricePaymentID,
			"invoice_id", paymentRecord.DestinationID,
			"payment_amount", amount.String())
		// Don't fail - invoice reconciliation is not critical for webhook success
	} else {
		h.logger.Infow("successfully reconciled payment with invoice",
			"flexprice_payment_id", flexpricePaymentID,
			"invoice_id", paymentRecord.DestinationID,
			"payment_amount", amount.String())
	}

	return nil
}

// handlePaymentFailed handles payment.failed webhook
func (h *Handler) handlePaymentFailed(ctx context.Context, event *RazorpayWebhookEvent, environmentID string, services *ServiceDependencies) error {
	payment := event.Payload.Payment.Entity

	h.logger.Infow("received payment.failed webhook",
		"razorpay_payment_id", payment.ID,
		"amount", payment.Amount,
		"currency", payment.Currency,
		"status", payment.Status,
		"error_code", payment.ErrorCode,
		"error_description", payment.ErrorDescription,
		"environment_id", environmentID,
	)

	// Get FlexPrice payment ID from notes
	flexpricePaymentID, ok := payment.Notes["flexprice_payment_id"].(string)
	if !ok || flexpricePaymentID == "" {
		h.logger.Warnw("no flexprice_payment_id found in payment notes",
			"razorpay_payment_id", payment.ID,
			"notes", payment.Notes)
		return nil // Not a FlexPrice-initiated payment
	}

	h.logger.Infow("processing FlexPrice payment failure",
		"razorpay_payment_id", payment.ID,
		"flexprice_payment_id", flexpricePaymentID)

	// Get payment record
	paymentRecord, err := services.PaymentService.GetPayment(ctx, flexpricePaymentID)
	if err != nil {
		h.logger.Errorw("failed to get payment record",
			"error", err,
			"flexprice_payment_id", flexpricePaymentID,
			"razorpay_payment_id", payment.ID)
		return nil // Don't fail webhook processing
	}

	if paymentRecord == nil {
		h.logger.Warnw("no payment record found",
			"flexprice_payment_id", flexpricePaymentID,
			"razorpay_payment_id", payment.ID)
		return nil
	}

	// Check if payment is already failed
	if paymentRecord.PaymentStatus == types.PaymentStatusFailed {
		h.logger.Infow("payment already marked as failed",
			"flexprice_payment_id", flexpricePaymentID,
			"razorpay_payment_id", payment.ID)
		return nil
	}

	// Build error message
	errorMsg := "Payment failed"
	if payment.ErrorDescription != "" {
		errorMsg = payment.ErrorDescription
	}

	// Update payment status to failed
	paymentStatus := string(types.PaymentStatusFailed)
	now := time.Now()

	// Prepare gateway metadata
	gatewayMetadata := types.Metadata{
		"razorpay_payment_id": payment.ID,
		"status":              payment.Status,
		"method":              payment.Method,
		"error_code":          payment.ErrorCode,
		"error_description":   payment.ErrorDescription,
		"error_source":        payment.ErrorSource,
		"error_step":          payment.ErrorStep,
		"error_reason":        payment.ErrorReason,
	}

	updateReq := dto.UpdatePaymentRequest{
		PaymentStatus: &paymentStatus,
		FailedAt:      &now,
		ErrorMessage:  &errorMsg,
		Metadata:      &gatewayMetadata,
	}

	_, err = services.PaymentService.UpdatePayment(ctx, flexpricePaymentID, updateReq)
	if err != nil {
		h.logger.Errorw("failed to update payment to failed",
			"error", err,
			"flexprice_payment_id", flexpricePaymentID,
			"razorpay_payment_id", payment.ID)
		return nil // Don't return error - webhook should always succeed
	}

	h.logger.Infow("updated payment to failed",
		"flexprice_payment_id", flexpricePaymentID,
		"razorpay_payment_id", payment.ID,
		"error_code", payment.ErrorCode,
		"error_description", payment.ErrorDescription)

	return nil
}

// reconcilePaymentWithInvoice updates the invoice payment status and amounts when a payment succeeds
func (h *Handler) reconcilePaymentWithInvoice(ctx context.Context, paymentID string, paymentAmount decimal.Decimal, paymentService interfaces.PaymentService, invoiceService interfaces.InvoiceService) error {
	h.logger.Infow("starting payment reconciliation with invoice",
		"payment_id", paymentID,
		"payment_amount", paymentAmount.String())

	// Get the payment record
	payment, err := paymentService.GetPayment(ctx, paymentID)
	if err != nil {
		h.logger.Errorw("failed to get payment record for reconciliation",
			"error", err,
			"payment_id", paymentID)
		return err
	}

	h.logger.Infow("got payment record for reconciliation",
		"payment_id", paymentID,
		"invoice_id", payment.DestinationID,
		"payment_amount", payment.Amount.String())

	// Get the invoice
	invoiceResp, err := invoiceService.GetInvoice(ctx, payment.DestinationID)
	if err != nil {
		h.logger.Errorw("failed to get invoice for payment reconciliation",
			"error", err,
			"payment_id", paymentID,
			"invoice_id", payment.DestinationID)
		return err
	}

	h.logger.Infow("got invoice for reconciliation",
		"payment_id", paymentID,
		"invoice_id", payment.DestinationID,
		"invoice_amount_due", invoiceResp.AmountDue.String(),
		"invoice_amount_paid", invoiceResp.AmountPaid.String(),
		"invoice_amount_remaining", invoiceResp.AmountRemaining.String(),
		"invoice_payment_status", invoiceResp.PaymentStatus,
		"invoice_status", invoiceResp.InvoiceStatus)

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

	h.logger.Infow("calculated new amounts for reconciliation",
		"payment_id", paymentID,
		"invoice_id", payment.DestinationID,
		"payment_amount", paymentAmount.String(),
		"new_amount_paid", newAmountPaid.String(),
		"new_amount_remaining", newAmountRemaining.String(),
		"new_payment_status", newPaymentStatus)

	// Update invoice payment status and amounts using reconciliation method
	h.logger.Infow("calling invoice reconciliation",
		"payment_id", paymentID,
		"invoice_id", payment.DestinationID,
		"payment_amount", paymentAmount.String(),
		"new_payment_status", newPaymentStatus)

	err = invoiceService.ReconcilePaymentStatus(ctx, payment.DestinationID, newPaymentStatus, &paymentAmount)
	if err != nil {
		h.logger.Errorw("failed to update invoice payment status during reconciliation",
			"error", err,
			"payment_id", paymentID,
			"invoice_id", payment.DestinationID,
			"payment_amount", paymentAmount.String(),
			"new_payment_status", newPaymentStatus)
		return err
	}

	h.logger.Infow("successfully reconciled payment with invoice",
		"payment_id", paymentID,
		"invoice_id", payment.DestinationID,
		"payment_amount", paymentAmount.String(),
		"new_payment_status", newPaymentStatus,
		"new_amount_paid", newAmountPaid.String(),
		"new_amount_remaining", newAmountRemaining.String())

	return nil
}
