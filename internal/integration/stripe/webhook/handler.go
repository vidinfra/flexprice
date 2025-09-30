package webhook

import (
	"context"
	"encoding/json"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/integration/stripe"
	"github.com/flexprice/flexprice/internal/interfaces"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
	stripeapi "github.com/stripe/stripe-go/v82"
)

// Handler handles Stripe webhook events
type Handler struct {
	client         *stripe.Client
	customerSvc    *stripe.CustomerService
	paymentSvc     *stripe.PaymentService
	invoiceSyncSvc *stripe.InvoiceSyncService
	logger         *logger.Logger
}

// NewHandler creates a new Stripe webhook handler
func NewHandler(
	client *stripe.Client,
	customerSvc *stripe.CustomerService,
	paymentSvc *stripe.PaymentService,
	invoiceSyncSvc *stripe.InvoiceSyncService,
	logger *logger.Logger,
) *Handler {
	return &Handler{
		client:         client,
		customerSvc:    customerSvc,
		paymentSvc:     paymentSvc,
		invoiceSyncSvc: invoiceSyncSvc,
		logger:         logger,
	}
}

// HandleWebhookEvent processes a Stripe webhook event
func (h *Handler) HandleWebhookEvent(ctx context.Context, event *stripeapi.Event, environmentID string, services *ServiceDependencies) error {
	h.logger.Infow("processing Stripe webhook event",
		"event_id", event.ID,
		"event_type", event.Type,
		"environment_id", environmentID,
	)

	switch string(event.Type) {
	case string(types.WebhookEventTypeCustomerCreated):
		return h.handleCustomerCreated(ctx, event, environmentID, services)
	case string(types.WebhookEventTypePaymentIntentSucceeded):
		return h.handlePaymentIntentSucceeded(ctx, event, environmentID, services)
	case string(types.WebhookEventTypeCheckoutSessionExpired):
		return h.handleCheckoutSessionExpired(ctx, event, environmentID, services)
	case string(types.WebhookEventTypePaymentIntentPaymentFailed):
		return h.handlePaymentIntentPaymentFailed(ctx, event, environmentID, services)
	case string(types.WebhookEventTypeSetupIntentSucceeded):
		return h.handleSetupIntentSucceeded(ctx, event, environmentID, services)
	default:
		h.logger.Infow("unhandled Stripe webhook event type", "type", event.Type)
		return nil // Not an error, just unhandled
	}
}

// ServiceDependencies contains all service dependencies needed by webhook handlers
type ServiceDependencies struct {
	CustomerService interfaces.CustomerService
	PaymentService  interfaces.PaymentService
	InvoiceService  interfaces.InvoiceService
}

// handleCustomerCreated handles customer.created webhook
func (h *Handler) handleCustomerCreated(ctx context.Context, event *stripeapi.Event, environmentID string, services *ServiceDependencies) error {
	// Parse webhook to get customer data
	var stripeCustomer stripeapi.Customer
	err := json.Unmarshal(event.Data.Raw, &stripeCustomer)
	if err != nil {
		h.logger.Errorw("failed to parse customer from webhook", "error", err)
		return ierr.NewError("failed to parse customer data").
			WithHint("Invalid customer data in webhook").
			Mark(ierr.ErrValidation)
	}

	h.logger.Infow("received customer.created webhook",
		"stripe_customer_id", stripeCustomer.ID,
		"customer_email", stripeCustomer.Email,
		"environment_id", environmentID,
		"event_id", event.ID,
	)

	// Create customer in our system from Stripe data
	err = h.customerSvc.CreateCustomerFromStripe(ctx, &stripeCustomer, environmentID, services.CustomerService)
	if err != nil {
		h.logger.Errorw("failed to create customer from Stripe webhook",
			"error", err,
			"stripe_customer_id", stripeCustomer.ID,
			"event_id", event.ID)
		return ierr.WithError(err).
			WithHint("Failed to create customer from Stripe webhook").
			Mark(ierr.ErrSystem)
	}

	h.logger.Infow("successfully created customer from Stripe webhook",
		"stripe_customer_id", stripeCustomer.ID,
		"customer_email", stripeCustomer.Email,
		"event_id", event.ID)

	return nil
}

// handlePaymentIntentSucceeded handles payment_intent.succeeded webhook
func (h *Handler) handlePaymentIntentSucceeded(ctx context.Context, event *stripeapi.Event, environmentID string, services *ServiceDependencies) error {
	// Parse webhook to get payment intent ID
	var webhookPaymentIntent stripeapi.PaymentIntent
	err := json.Unmarshal(event.Data.Raw, &webhookPaymentIntent)
	if err != nil {
		h.logger.Errorw("failed to parse payment intent from webhook", "error", err)
		return ierr.NewError("failed to parse payment intent data").
			WithHint("Invalid payment intent data in webhook").
			Mark(ierr.ErrValidation)
	}

	paymentIntentID := webhookPaymentIntent.ID
	h.logger.Infow("received payment_intent.succeeded webhook",
		"payment_intent_id", paymentIntentID,
		"environment_id", environmentID,
		"event_id", event.ID,
		"event_type", event.Type,
	)

	// Fetch the latest payment intent data from Stripe API instead of relying on webhook data
	paymentIntent, err := h.paymentSvc.GetPaymentIntent(ctx, paymentIntentID, environmentID)
	if err != nil {
		h.logger.Errorw("failed to fetch payment intent from Stripe API",
			"error", err,
			"payment_intent_id", paymentIntentID,
			"event_id", event.ID)
		return ierr.WithError(err).
			WithHint("Failed to fetch payment intent from Stripe").
			Mark(ierr.ErrSystem)
	}

	h.logger.Infow("fetched payment intent from Stripe API",
		"payment_intent_id", paymentIntent.ID,
		"status", paymentIntent.Status,
		"amount", paymentIntent.Amount,
		"currency", paymentIntent.Currency,
		"metadata", paymentIntent.Metadata,
	)

	// Check payment source to determine processing approach
	paymentSource := paymentIntent.Metadata["payment_source"]
	paymentType := paymentIntent.Metadata["payment_type"]

	if paymentSource == "flexprice" && paymentType == "checkout" {
		return h.handleFlexPriceCheckoutPayment(ctx, paymentIntent, environmentID, services)
	} else {
		return h.handleFlexPriceDirectPayment(ctx, paymentIntent, environmentID, services)
	}
}

// handleFlexPriceCheckoutPayment handles payment intents from FlexPrice checkout sessions
func (h *Handler) handleFlexPriceCheckoutPayment(ctx context.Context, paymentIntent *stripeapi.PaymentIntent, environmentID string, services *ServiceDependencies) error {
	// Get flexprice_payment_id from metadata
	flexpricePaymentID := ""
	if paymentID, exists := paymentIntent.Metadata["flexprice_payment_id"]; exists {
		flexpricePaymentID = paymentID
	}

	if flexpricePaymentID == "" {
		h.logger.Warnw("no flexprice_payment_id found in payment intent metadata",
			"payment_intent_id", paymentIntent.ID)
		return nil
	}

	h.logger.Infow("processing FlexPrice checkout payment",
		"payment_intent_id", paymentIntent.ID,
		"flexprice_payment_id", flexpricePaymentID,
		"amount", paymentIntent.Amount,
		"currency", paymentIntent.Currency)

	// Get payment record by flexprice_payment_id
	payment, err := services.PaymentService.GetPayment(ctx, flexpricePaymentID)
	if err != nil {
		h.logger.Errorw("failed to get payment record",
			"error", err,
			"flexprice_payment_id", flexpricePaymentID,
			"payment_intent_id", paymentIntent.ID)
		return ierr.WithError(err).
			WithHint("Failed to get payment record").
			Mark(ierr.ErrSystem)
	}

	if payment == nil {
		h.logger.Warnw("no payment record found", "flexprice_payment_id", flexpricePaymentID, "payment_intent_id", paymentIntent.ID)
		return nil
	}

	h.logger.Infow("found payment record", "payment_id", payment.ID, "flexprice_payment_id", flexpricePaymentID)

	// Check if payment is already successful - prevent any updates to successful payments
	if payment.PaymentStatus == types.PaymentStatusSucceeded {
		h.logger.Warnw("payment is already successful, skipping update",
			"payment_id", payment.ID,
			"flexprice_payment_id", flexpricePaymentID,
			"payment_intent_id", paymentIntent.ID,
			"current_status", payment.PaymentStatus)
		return nil
	}

	// Get payment status from Stripe API
	paymentStatusResp, err := h.paymentSvc.GetPaymentStatusByPaymentIntent(ctx, paymentIntent.ID, environmentID)
	if err != nil {
		h.logger.Errorw("failed to get payment status from Stripe API",
			"error", err,
			"payment_intent_id", paymentIntent.ID)
		return ierr.WithError(err).
			WithHint("Failed to get payment status from Stripe").
			Mark(ierr.ErrSystem)
	}

	h.logger.Infow("retrieved payment status from Stripe API",
		"payment_intent_id", paymentIntent.ID,
		"payment_method_id", paymentStatusResp.PaymentMethodID,
		"status", paymentStatusResp.Status,
		"amount", paymentStatusResp.Amount.String(),
		"currency", paymentStatusResp.Currency,
	)

	// Update payment record based on Stripe API response
	var paymentStatus string
	var failedAt *time.Time
	var errorMessage *string

	switch paymentStatusResp.Status {
	case "succeeded":
		paymentStatus = string(types.PaymentStatusSucceeded)
		// Set payment method ID if available and save_card_and_make_default was requested
		if paymentStatusResp.PaymentMethodID != "" && payment.Metadata != nil {
			if saveCard, exists := payment.Metadata["save_card_and_make_default"]; exists && saveCard == "true" {
				// Get customer ID from invoice
				invoiceResp, err := services.InvoiceService.GetInvoice(ctx, payment.DestinationID)
				if err != nil {
					h.logger.Errorw("failed to get invoice for customer ID",
						"error", err,
						"payment_id", payment.ID,
						"invoice_id", payment.DestinationID)
				} else {
					h.logger.Infow("setting payment method as default for customer",
						"payment_id", payment.ID,
						"customer_id", invoiceResp.CustomerID,
						"payment_method_id", paymentStatusResp.PaymentMethodID)

					err := h.paymentSvc.SetDefaultPaymentMethod(ctx, invoiceResp.CustomerID, paymentStatusResp.PaymentMethodID, services.CustomerService)
					if err != nil {
						h.logger.Errorw("failed to set default payment method",
							"error", err,
							"payment_id", payment.ID,
							"customer_id", invoiceResp.CustomerID,
							"payment_method_id", paymentStatusResp.PaymentMethodID)
						// Don't fail the entire webhook processing
					} else {
						h.logger.Infow("successfully set default payment method",
							"payment_id", payment.ID,
							"customer_id", invoiceResp.CustomerID,
							"payment_method_id", paymentStatusResp.PaymentMethodID)
					}
				}
			}
		}
	case "requires_payment_method", "requires_confirmation", "requires_action", "processing", "requires_capture":
		paymentStatus = string(types.PaymentStatusPending)
	case "canceled":
		paymentStatus = string(types.PaymentStatusFailed)
		now := time.Now()
		failedAt = &now
		errorMsg := "Payment was canceled"
		errorMessage = &errorMsg
	default:
		paymentStatus = string(types.PaymentStatusFailed)
		now := time.Now()
		failedAt = &now
		errorMsg := "Payment failed with status: " + paymentStatusResp.Status
		errorMessage = &errorMsg
	}

	// Update payment record
	updateReq := dto.UpdatePaymentRequest{
		PaymentStatus: &paymentStatus,
		FailedAt:      failedAt,
		ErrorMessage:  errorMessage,
	}

	if paymentStatusResp.PaymentMethodID != "" {
		updateReq.PaymentMethodID = &paymentStatusResp.PaymentMethodID
	}

	_, err = services.PaymentService.UpdatePayment(ctx, payment.ID, updateReq)
	if err != nil {
		h.logger.Errorw("failed to update payment record",
			"error", err,
			"payment_id", payment.ID,
			"new_status", paymentStatus)
		return ierr.WithError(err).
			WithHint("Failed to update payment record").
			Mark(ierr.ErrSystem)
	}

	h.logger.Infow("successfully updated payment record",
		"payment_id", payment.ID,
		"new_status", paymentStatus,
		"flexprice_payment_id", flexpricePaymentID)

	// If payment succeeded, attach to Stripe invoice if available and reconcile with FlexPrice invoice
	if paymentStatus == string(types.PaymentStatusSucceeded) {
		// Check if we have a Stripe invoice ID in metadata
		if stripeInvoiceID, exists := paymentIntent.Metadata["stripe_invoice_id"]; exists && stripeInvoiceID != "" {
			h.logger.Infow("attempting to attach payment to Stripe invoice",
				"payment_id", payment.ID,
				"payment_intent_id", paymentIntent.ID,
				"stripe_invoice_id", stripeInvoiceID)

			// Get Stripe client
			stripeClient, _, err := h.client.GetStripeClient(ctx)
			if err != nil {
				h.logger.Errorw("failed to get Stripe client for invoice attachment",
					"error", err,
					"payment_id", payment.ID)
			} else {
				// Attach payment to Stripe invoice
				err = h.paymentSvc.AttachPaymentToStripeInvoice(ctx, stripeClient, paymentIntent.ID, stripeInvoiceID)
				if err != nil {
					h.logger.Errorw("failed to attach payment to Stripe invoice",
						"error", err,
						"payment_id", payment.ID,
						"payment_intent_id", paymentIntent.ID,
						"stripe_invoice_id", stripeInvoiceID)
					// Don't fail the entire webhook processing
				} else {
					h.logger.Infow("successfully attached payment to Stripe invoice",
						"payment_id", payment.ID,
						"payment_intent_id", paymentIntent.ID,
						"stripe_invoice_id", stripeInvoiceID)
				}
			}
		} else {
			h.logger.Debugw("no Stripe invoice ID in metadata, skipping attachment",
				"payment_id", payment.ID,
				"payment_intent_id", paymentIntent.ID)
		}

		// Reconcile with FlexPrice invoice
		err = h.paymentSvc.ReconcilePaymentWithInvoice(ctx, payment.ID, paymentStatusResp.Amount, services.PaymentService, services.InvoiceService)
		if err != nil {
			h.logger.Errorw("failed to reconcile payment with invoice",
				"error", err,
				"payment_id", payment.ID,
				"amount", paymentStatusResp.Amount.String())
			// Don't fail the entire webhook processing
		} else {
			h.logger.Infow("successfully reconciled payment with invoice",
				"payment_id", payment.ID,
				"amount", paymentStatusResp.Amount.String())
		}
	}

	return nil
}

// handleFlexPriceDirectPayment handles direct payment intents from FlexPrice (card charges)
func (h *Handler) handleFlexPriceDirectPayment(ctx context.Context, paymentIntent *stripeapi.PaymentIntent, environmentID string, services *ServiceDependencies) error {
	h.logger.Infow("processing FlexPrice direct payment",
		"payment_intent_id", paymentIntent.ID,
		"amount", paymentIntent.Amount,
		"currency", paymentIntent.Currency)

	// For direct payments, we need to find the payment record by payment intent ID
	payment, err := h.findPaymentByPaymentIntentID(ctx, paymentIntent.ID, services.PaymentService)
	if err != nil {
		h.logger.Errorw("failed to find payment record by payment intent ID",
			"error", err,
			"payment_intent_id", paymentIntent.ID)
		return ierr.WithError(err).
			WithHint("Failed to find payment record").
			Mark(ierr.ErrSystem)
	}

	if payment == nil {
		h.logger.Warnw("no payment record found for payment intent", "payment_intent_id", paymentIntent.ID)
		return nil
	}

	h.logger.Infow("found payment record for direct payment", "payment_id", payment.ID, "payment_intent_id", paymentIntent.ID)

	// Check if payment is already successful
	if payment.PaymentStatus == types.PaymentStatusSucceeded {
		h.logger.Warnw("payment is already successful, skipping update",
			"payment_id", payment.ID,
			"payment_intent_id", paymentIntent.ID,
			"current_status", payment.PaymentStatus)
		return nil
	}

	// Update payment status to succeeded
	paymentStatus := string(types.PaymentStatusSucceeded)
	updateReq := dto.UpdatePaymentRequest{
		PaymentStatus: &paymentStatus,
	}

	_, err = services.PaymentService.UpdatePayment(ctx, payment.ID, updateReq)
	if err != nil {
		h.logger.Errorw("failed to update payment record",
			"error", err,
			"payment_id", payment.ID,
			"new_status", paymentStatus)
		return ierr.WithError(err).
			WithHint("Failed to update payment record").
			Mark(ierr.ErrSystem)
	}

	h.logger.Infow("successfully updated payment record for direct payment",
		"payment_id", payment.ID,
		"new_status", paymentStatus,
		"payment_intent_id", paymentIntent.ID)

	// Reconcile with invoice
	amount := decimal.NewFromInt(paymentIntent.Amount).Div(decimal.NewFromInt(100))
	err = h.paymentSvc.ReconcilePaymentWithInvoice(ctx, payment.ID, amount, services.PaymentService, services.InvoiceService)
	if err != nil {
		h.logger.Errorw("failed to reconcile direct payment with invoice",
			"error", err,
			"payment_id", payment.ID,
			"amount", amount.String())
		// Don't fail the entire webhook processing
	} else {
		h.logger.Infow("successfully reconciled direct payment with invoice",
			"payment_id", payment.ID,
			"amount", amount.String())
	}

	return nil
}

// handleCheckoutSessionExpired handles checkout.session.expired webhook
func (h *Handler) handleCheckoutSessionExpired(ctx context.Context, event *stripeapi.Event, environmentID string, services *ServiceDependencies) error {
	// Parse webhook to get session data
	var session stripeapi.CheckoutSession
	err := json.Unmarshal(event.Data.Raw, &session)
	if err != nil {
		h.logger.Errorw("failed to parse checkout session from webhook", "error", err)
		return ierr.NewError("failed to parse checkout session data").
			WithHint("Invalid checkout session data in webhook").
			Mark(ierr.ErrValidation)
	}

	sessionID := session.ID
	h.logger.Infow("received checkout.session.expired webhook",
		"session_id", sessionID,
		"environment_id", environmentID,
		"event_id", event.ID)

	// Find payment record by session ID
	payment, err := h.findPaymentBySessionID(ctx, sessionID, services.PaymentService)
	if err != nil {
		h.logger.Errorw("failed to find payment record",
			"error", err,
			"session_id", sessionID)
		return ierr.WithError(err).
			WithHint("Failed to find payment record").
			Mark(ierr.ErrSystem)
	}

	if payment == nil {
		h.logger.Warnw("no payment record found for expired session", "session_id", sessionID)
		return nil
	}

	// Only update if payment is still pending
	if payment.PaymentStatus != types.PaymentStatusPending && payment.PaymentStatus != types.PaymentStatusInitiated {
		h.logger.Infow("payment status is not pending, skipping expiration update",
			"payment_id", payment.ID,
			"session_id", sessionID,
			"current_status", payment.PaymentStatus)
		return nil
	}

	// Update payment status to expired
	paymentStatus := string(types.PaymentStatusFailed)
	now := time.Now()
	errorMsg := "Checkout session expired"
	updateReq := dto.UpdatePaymentRequest{
		PaymentStatus: &paymentStatus,
		FailedAt:      &now,
		ErrorMessage:  &errorMsg,
	}

	_, err = services.PaymentService.UpdatePayment(ctx, payment.ID, updateReq)
	if err != nil {
		h.logger.Errorw("failed to update payment record for expired session",
			"error", err,
			"payment_id", payment.ID,
			"session_id", sessionID)
		return ierr.WithError(err).
			WithHint("Failed to update payment record").
			Mark(ierr.ErrSystem)
	}

	h.logger.Infow("successfully updated payment record for expired session",
		"payment_id", payment.ID,
		"session_id", sessionID,
		"new_status", paymentStatus)

	return nil
}

// handlePaymentIntentPaymentFailed handles payment_intent.payment_failed webhook
func (h *Handler) handlePaymentIntentPaymentFailed(ctx context.Context, event *stripeapi.Event, environmentID string, services *ServiceDependencies) error {
	// Parse webhook to get payment intent data
	var paymentIntent stripeapi.PaymentIntent
	err := json.Unmarshal(event.Data.Raw, &paymentIntent)
	if err != nil {
		h.logger.Errorw("failed to parse payment intent from webhook", "error", err)
		return ierr.NewError("failed to parse payment intent data").
			WithHint("Invalid payment intent data in webhook").
			Mark(ierr.ErrValidation)
	}

	paymentIntentID := paymentIntent.ID
	h.logger.Infow("received payment_intent.payment_failed webhook",
		"payment_intent_id", paymentIntentID,
		"environment_id", environmentID,
		"event_id", event.ID)

	// Check if this is a FlexPrice payment
	paymentSource := paymentIntent.Metadata["payment_source"]
	if paymentSource != "flexprice" {
		h.logger.Infow("ignoring payment intent failure from external source",
			"payment_intent_id", paymentIntentID,
			"payment_source", paymentSource)
		return nil
	}

	// Find payment record
	var payment *dto.PaymentResponse
	paymentType := paymentIntent.Metadata["payment_type"]
	if paymentType == "checkout" {
		// Find by session ID for checkout payments
		sessionID := paymentIntent.Metadata["session_id"]
		if sessionID != "" {
			payment, err = h.findPaymentBySessionID(ctx, sessionID, services.PaymentService)
		}
	} else {
		// Find by payment intent ID for direct payments
		payment, err = h.findPaymentByPaymentIntentID(ctx, paymentIntentID, services.PaymentService)
	}

	if err != nil {
		h.logger.Errorw("failed to find payment record",
			"error", err,
			"payment_intent_id", paymentIntentID)
		return ierr.WithError(err).
			WithHint("Failed to find payment record").
			Mark(ierr.ErrSystem)
	}

	if payment == nil {
		h.logger.Warnw("no payment record found for failed payment intent", "payment_intent_id", paymentIntentID)
		return nil
	}

	// Update payment status to failed
	paymentStatus := string(types.PaymentStatusFailed)
	now := time.Now()
	errorMsg := "Payment failed"
	if paymentIntent.LastPaymentError != nil && paymentIntent.LastPaymentError.Msg != "" {
		errorMsg = paymentIntent.LastPaymentError.Msg
	}

	updateReq := dto.UpdatePaymentRequest{
		PaymentStatus: &paymentStatus,
		FailedAt:      &now,
		ErrorMessage:  &errorMsg,
	}

	_, err = services.PaymentService.UpdatePayment(ctx, payment.ID, updateReq)
	if err != nil {
		h.logger.Errorw("failed to update payment record for failed payment",
			"error", err,
			"payment_id", payment.ID,
			"payment_intent_id", paymentIntentID)
		return ierr.WithError(err).
			WithHint("Failed to update payment record").
			Mark(ierr.ErrSystem)
	}

	h.logger.Infow("successfully updated payment record for failed payment",
		"payment_id", payment.ID,
		"payment_intent_id", paymentIntentID,
		"new_status", paymentStatus,
		"error_message", errorMsg)

	return nil
}

// handleSetupIntentSucceeded handles setup_intent.succeeded webhook
func (h *Handler) handleSetupIntentSucceeded(ctx context.Context, event *stripeapi.Event, environmentID string, services *ServiceDependencies) error {
	// Parse webhook to get setup intent data
	var setupIntent stripeapi.SetupIntent
	err := json.Unmarshal(event.Data.Raw, &setupIntent)
	if err != nil {
		h.logger.Errorw("failed to parse setup intent from webhook", "error", err)
		return ierr.NewError("failed to parse setup intent data").
			WithHint("Invalid setup intent data in webhook").
			Mark(ierr.ErrValidation)
	}

	h.logger.Infow("received setup_intent.succeeded webhook",
		"setup_intent_id", setupIntent.ID,
		"customer_id", func() string {
			if setupIntent.Customer != nil {
				return setupIntent.Customer.ID
			}
			return ""
		}(),
		"payment_method_id", func() string {
			if setupIntent.PaymentMethod != nil {
				return setupIntent.PaymentMethod.ID
			}
			return ""
		}(),
		"environment_id", environmentID,
		"event_id", event.ID)

	// For setup intents, we typically just log the success
	// The payment method is already attached to the customer in Stripe
	// If we need to do anything specific (like updating our records), we can do it here

	h.logger.Infow("setup intent succeeded - payment method saved",
		"setup_intent_id", setupIntent.ID,
		"customer_id", func() string {
			if setupIntent.Customer != nil {
				return setupIntent.Customer.ID
			}
			return ""
		}(),
		"payment_method_id", func() string {
			if setupIntent.PaymentMethod != nil {
				return setupIntent.PaymentMethod.ID
			}
			return ""
		}())

	return nil
}

// Helper methods

// findPaymentBySessionID finds a payment record by Stripe session ID
func (h *Handler) findPaymentBySessionID(ctx context.Context, sessionID string, paymentService interfaces.PaymentService) (*dto.PaymentResponse, error) {
	filter := types.NewNoLimitPaymentFilter()
	// Set a reasonable limit for searching
	if filter.QueryFilter != nil {
		limit := 50
		filter.QueryFilter.Limit = &limit
	}

	payments, err := paymentService.ListPayments(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Find payment with matching session ID
	for _, payment := range payments.Items {
		if payment.GatewayTrackingID != nil && *payment.GatewayTrackingID == sessionID {
			return payment, nil
		}
	}

	return nil, nil // Not found
}

// findPaymentByPaymentIntentID finds a payment record by Stripe payment intent ID
func (h *Handler) findPaymentByPaymentIntentID(ctx context.Context, paymentIntentID string, paymentService interfaces.PaymentService) (*dto.PaymentResponse, error) {
	filter := types.NewNoLimitPaymentFilter()
	// Set a reasonable limit for searching
	if filter.QueryFilter != nil {
		limit := 50
		filter.QueryFilter.Limit = &limit
	}

	payments, err := paymentService.ListPayments(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Find payment with matching payment intent ID
	for _, payment := range payments.Items {
		if payment.GatewayPaymentID != nil && *payment.GatewayPaymentID == paymentIntentID {
			return payment, nil
		}
	}

	return nil, nil // Not found
}
