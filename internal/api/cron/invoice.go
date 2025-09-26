package cron

import (
	"context"
	"net/http"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/service"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v82"
)

// InvoiceHandler handles invoice related cron jobs
type InvoiceHandler struct {
	invoiceService      service.InvoiceService
	subscriptionService service.SubscriptionService
	connectionService   service.ConnectionService
	tenantService       service.TenantService
	environmentService  service.EnvironmentService
	stripeService       *service.StripeInvoiceSyncService
	logger              *logger.Logger
}

// NewInvoiceHandler creates a new invoice handler
func NewInvoiceHandler(
	invoiceService service.InvoiceService,
	subscriptionService service.SubscriptionService,
	connectionService service.ConnectionService,
	tenantService service.TenantService,
	environmentService service.EnvironmentService,
	stripeService *service.StripeInvoiceSyncService,
	logger *logger.Logger,
) *InvoiceHandler {
	return &InvoiceHandler{
		invoiceService:      invoiceService,
		subscriptionService: subscriptionService,
		connectionService:   connectionService,
		tenantService:       tenantService,
		environmentService:  environmentService,
		stripeService:       stripeService,
		logger:              logger,
	}
}

// TenantEnvironmentPair represents a tenant and environment pair to process
type TenantEnvironmentPair struct {
	TenantID      string `json:"tenant_id" binding:"required"`
	EnvironmentID string `json:"environment_id" binding:"required"`
}

// VoidOldPendingInvoicesRequest represents the request payload for the void old pending invoices cron job
type VoidOldPendingInvoicesRequest struct {
	// targets is an optional array of tenant-environment pairs to process. If empty, all tenants and environments are processed.
	Targets []TenantEnvironmentPair `json:"targets,omitempty"`
}

// VoidOldPendingInvoices processes incomplete subscriptions and voids their old pending invoices
func (h *InvoiceHandler) VoidOldPendingInvoices(c *gin.Context) {
	h.logger.Infow("starting void old pending invoices cron job", "time", time.Now().UTC().Format(time.RFC3339))

	ctx := c.Request.Context()

	// Parse request parameters (optional)
	var req VoidOldPendingInvoicesRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBind(&req); err != nil {
			h.logger.Errorw("failed to parse request parameters", "error", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request parameters"})
			return
		}
	}
	// If no body provided, req.Targets will be empty (default behavior)

	response := &dto.VoidOldPendingInvoicesResponse{
		Items:   make([]*dto.VoidOldPendingInvoicesResponseItem, 0),
		Total:   0,
		Success: 0,
		Failed:  0,
	}

	// Log filtering parameters
	if len(req.Targets) > 0 {
		h.logger.Infow("filtering enabled", "targets", req.Targets)
	}

	if len(req.Targets) == 0 {
		// No specific targets, process all tenants and environments
		err := h.processAllTenantsAndEnvironments(ctx, response)
		if err != nil {
			h.logger.Errorw("failed to process all tenants and environments", "error", err)
			_ = c.Error(err)
			return
		}
	} else {
		// Process specific tenant-environment pairs
		for _, target := range req.Targets {
			h.logger.Infow("processing target",
				"tenant_id", target.TenantID,
				"environment_id", target.EnvironmentID)

			// Create context with tenant and environment
			tenantCtx := context.WithValue(ctx, types.CtxTenantID, target.TenantID)
			envCtx := context.WithValue(tenantCtx, types.CtxEnvironmentID, target.EnvironmentID)

			// Process incomplete subscriptions for this specific environment
			envResponse, err := h.processIncompleteSubscriptionsForEnvironment(envCtx, target.TenantID, target.EnvironmentID)
			if err != nil {
				h.logger.Errorw("failed to process incomplete subscriptions for target",
					"tenant_id", target.TenantID,
					"environment_id", target.EnvironmentID,
					"error", err)
				response.Failed++
				continue
			}

			response.Items = append(response.Items, envResponse)
			response.Total += envResponse.Count
			response.Success += envResponse.Success
			response.Failed += envResponse.Failed
		}
	}

	h.logger.Infow("completed void old pending invoices cron job",
		"total_processed", response.Total,
		"successful", response.Success,
		"failed", response.Failed)

	c.JSON(http.StatusOK, response)
}

// processAllTenantsAndEnvironments processes all tenants and their environments
func (h *InvoiceHandler) processAllTenantsAndEnvironments(ctx context.Context, response *dto.VoidOldPendingInvoicesResponse) error {
	// Get all tenants
	tenants, err := h.tenantService.GetAllTenants(ctx)
	if err != nil {
		return err
	}

	h.logger.Infow("processing all tenants", "count", len(tenants))

	// Process each tenant
	for _, tenant := range tenants {
		h.logger.Infow("processing tenant", "tenant_id", tenant.ID, "name", tenant.Name)

		tenantCtx := context.WithValue(ctx, types.CtxTenantID, tenant.ID)

		// Get all environments for this tenant
		environments, err := h.environmentService.GetEnvironments(tenantCtx, types.GetDefaultFilter())
		if err != nil {
			h.logger.Errorw("failed to get environments for tenant",
				"tenant_id", tenant.ID, "error", err)
			response.Failed++
			continue
		}

		for _, environment := range environments.Environments {
			h.logger.Infow("processing environment",
				"tenant_id", tenant.ID,
				"environment_id", environment.ID,
				"name", environment.Name)

			envCtx := context.WithValue(tenantCtx, types.CtxEnvironmentID, environment.ID)

			// Process incomplete subscriptions for this environment
			envResponse, err := h.processIncompleteSubscriptionsForEnvironment(envCtx, tenant.ID, environment.ID)
			if err != nil {
				h.logger.Errorw("failed to process incomplete subscriptions for environment",
					"tenant_id", tenant.ID,
					"environment_id", environment.ID,
					"error", err)
				response.Failed++
				continue
			}

			response.Items = append(response.Items, envResponse)
			response.Total += envResponse.Count
			response.Success += envResponse.Success
			response.Failed += envResponse.Failed
		}
	}

	return nil
}

// processIncompleteSubscriptionsForEnvironment processes incomplete subscriptions for a specific environment
func (h *InvoiceHandler) processIncompleteSubscriptionsForEnvironment(
	ctx context.Context,
	tenantID, environmentID string,
) (*dto.VoidOldPendingInvoicesResponseItem, error) {

	response := &dto.VoidOldPendingInvoicesResponseItem{
		TenantID:      tenantID,
		EnvironmentID: environmentID,
		Count:         0,
	}

	// Get all incomplete subscriptions for this environment
	subscriptionFilter := &types.SubscriptionFilter{
		SubscriptionStatus: []types.SubscriptionStatus{
			types.SubscriptionStatusIncomplete,
		},
	}

	subscriptions, err := h.subscriptionService.ListSubscriptions(ctx, subscriptionFilter)
	if err != nil {
		return response, err
	}

	h.logger.Infow("found incomplete subscriptions",
		"tenant_id", tenantID,
		"environment_id", environmentID,
		"count", len(subscriptions.Items))

	// Process each incomplete subscription
	for _, sub := range subscriptions.Items {
		h.logger.Infow("processing subscription",
			"subscription_id", sub.ID,
			"customer_id", sub.CustomerID,
			"created_at", sub.CreatedAt)

		if err := h.processSubscriptionOldPendingInvoices(ctx, sub); err != nil {
			h.logger.Errorw("failed to process subscription old pending invoices",
				"subscription_id", sub.ID,
				"error", err)
			response.Failed++
		} else {
			response.Success++
		}
		response.Count++
	}

	return response, nil
}

// processSubscriptionOldPendingInvoices processes old pending invoices for a specific subscription
func (h *InvoiceHandler) processSubscriptionOldPendingInvoices(
	ctx context.Context,
	sub *dto.SubscriptionResponse,
) error {

	h.logger.Infow("starting to process subscription for old pending invoices",
		"subscription_id", sub.ID,
		"subscription_status", sub.SubscriptionStatus,
		"customer_id", sub.CustomerID,
		"created_at", sub.CreatedAt)

	// Calculate cutoff time (24 hours ago)
	cutoffTime := time.Now().UTC().Add(-24 * time.Hour)

	h.logger.Infow("searching for old pending invoices",
		"subscription_id", sub.ID,
		"cutoff_time", cutoffTime.Format(time.RFC3339),
		"looking_for_invoices_older_than", "24 hours")

	// Get pending invoices for this subscription that are older than 24 hours
	// Note: We'll filter by creation date
	invoiceFilter := &types.InvoiceFilter{
		SubscriptionID: sub.ID,
		InvoiceStatus: []types.InvoiceStatus{
			types.InvoiceStatusDraft,
			types.InvoiceStatusFinalized,
		},
		PaymentStatus: []types.PaymentStatus{
			types.PaymentStatusPending,
			types.PaymentStatusFailed,
		},
		// Removed TimeRangeFilter since it uses period_end, not created_at
	}

	invoices, err := h.invoiceService.ListInvoices(ctx, invoiceFilter)
	if err != nil {
		h.logger.Errorw("failed to list invoices for subscription",
			"subscription_id", sub.ID,
			"error", err)
		return err
	}

	h.logger.Infow("found pending invoices for subscription (before age filtering)",
		"subscription_id", sub.ID,
		"count", len(invoices.Items),
		"cutoff_time", cutoffTime.Format(time.RFC3339))

	// Filter invoices by creation date (older than 24 hours)
	var oldInvoices []*dto.InvoiceResponse
	for _, inv := range invoices.Items {
		h.logger.Infow("checking invoice age",
			"subscription_id", sub.ID,
			"invoice_id", inv.ID,
			"created_at", inv.CreatedAt,
			"cutoff_time", cutoffTime.Format(time.RFC3339),
			"is_old", inv.CreatedAt.Before(cutoffTime))

		if inv.CreatedAt.Before(cutoffTime) {
			oldInvoices = append(oldInvoices, inv)
			h.logger.Infow("invoice is old enough - including",
				"subscription_id", sub.ID,
				"invoice_id", inv.ID,
				"invoice_status", inv.InvoiceStatus,
				"payment_status", inv.PaymentStatus,
				"created_at", inv.CreatedAt,
				"amount_due", inv.AmountDue,
				"amount_paid", inv.AmountPaid,
				"total", inv.Total)
		} else {
			h.logger.Infow("invoice is too recent - skipping",
				"subscription_id", sub.ID,
				"invoice_id", inv.ID,
				"created_at", inv.CreatedAt)
		}
	}

	h.logger.Infow("found old pending invoices for subscription (after age filtering)",
		"subscription_id", sub.ID,
		"total_invoices", len(invoices.Items),
		"old_invoices_count", len(oldInvoices),
		"cutoff_time", cutoffTime.Format(time.RFC3339))

	if len(oldInvoices) == 0 {
		h.logger.Infow("no old pending invoices found for subscription",
			"subscription_id", sub.ID)
		return nil
	}

	// Check if subscription has any invoices synced to Stripe
	var hasAnyStripeSyncedInvoices bool
	var invoiceToVoid *dto.InvoiceResponse
	var hasAnyPartialPayments bool

	h.logger.Infow("checking if subscription has any Stripe-synced invoices",
		"subscription_id", sub.ID,
		"old_invoices_count", len(oldInvoices))

	for _, inv := range oldInvoices {
		// Check if this invoice is synced to Stripe
		mapping, err := h.stripeService.GetExistingStripeMapping(ctx, inv.ID)
		if err == nil && mapping != nil {
			hasAnyStripeSyncedInvoices = true
			h.logger.Infow("found Stripe-synced invoice",
				"invoice_id", inv.ID,
				"subscription_id", sub.ID,
				"stripe_invoice_id", mapping.ProviderEntityID)
		} else {
			h.logger.Infow("invoice not synced to Stripe",
				"invoice_id", inv.ID,
				"subscription_id", sub.ID)
			continue // Skip non-Stripe invoices
		}

		// Check if invoice has any partial payments in FlexPrice
		if inv.AmountPaid.IsPositive() {
			h.logger.Infow("found invoice with partial payment in FlexPrice",
				"invoice_id", inv.ID,
				"subscription_id", sub.ID,
				"amount_paid", inv.AmountPaid,
				"total", inv.Total,
				"amount_remaining", inv.AmountRemaining)
			hasAnyPartialPayments = true
			continue
		}

		// Check if invoice has partial payments in Stripe
		if hasStripePartialPayment, err := h.checkStripePartialPayment(ctx, inv.ID); err != nil {
			h.logger.Errorw("failed to check Stripe partial payment status",
				"invoice_id", inv.ID,
				"subscription_id", sub.ID,
				"error", err)
			continue
		} else if hasStripePartialPayment {
			h.logger.Infow("found invoice with partial payment in Stripe",
				"invoice_id", inv.ID,
				"subscription_id", sub.ID)
			hasAnyPartialPayments = true
			continue
		}

		// This invoice has no partial payments and is synced to Stripe, we can void it
		if invoiceToVoid == nil {
			invoiceToVoid = inv
			break // Take the first eligible invoice (oldest)
		}
	}

	// If no invoices are synced to Stripe, skip this subscription
	if !hasAnyStripeSyncedInvoices {
		h.logger.Infow("subscription has no Stripe-synced invoices, skipping",
			"subscription_id", sub.ID,
			"total_old_invoices", len(oldInvoices))
		return nil
	}

	// If any invoice has partial payments, don't void anything or cancel subscription
	if hasAnyPartialPayments {
		h.logger.Infow("subscription has invoices with partial payments, skipping cancellation",
			"subscription_id", sub.ID,
			"total_invoices", len(oldInvoices))
		return nil
	}

	// If no eligible invoice found, log and return
	if invoiceToVoid == nil {
		h.logger.Infow("no eligible invoices found for voiding",
			"subscription_id", sub.ID,
			"total_invoices", len(oldInvoices))
		return nil
	}

	h.logger.Infow("proceeding to void selected invoice and cancel subscription",
		"subscription_id", sub.ID,
		"invoice_to_void", invoiceToVoid.ID,
		"invoice_amount", invoiceToVoid.AmountDue,
		"invoice_created_at", invoiceToVoid.CreatedAt)

	// Void the selected invoice
	if err := h.voidOldPendingInvoice(ctx, invoiceToVoid); err != nil {
		h.logger.Errorw("failed to void old pending invoice",
			"invoice_id", invoiceToVoid.ID,
			"subscription_id", sub.ID,
			"error", err)
		return err
	}

	h.logger.Infow("successfully voided old pending invoice",
		"invoice_id", invoiceToVoid.ID,
		"subscription_id", sub.ID,
		"amount", invoiceToVoid.AmountDue)

	// Cancel the subscription
	h.logger.Infow("proceeding to cancel incomplete subscription",
		"subscription_id", sub.ID,
		"customer_id", sub.CustomerID)

	if err := h.cancelIncompleteSubscription(ctx, sub); err != nil {
		h.logger.Errorw("failed to cancel incomplete subscription",
			"subscription_id", sub.ID,
			"error", err)
		return err
	}

	h.logger.Infow("successfully cancelled incomplete subscription",
		"subscription_id", sub.ID,
		"customer_id", sub.CustomerID)

	return nil
}

// cancelIncompleteSubscription cancels an incomplete subscription that has old pending invoices
func (h *InvoiceHandler) cancelIncompleteSubscription(ctx context.Context, sub *dto.SubscriptionResponse) error {
	h.logger.Infow("cancelling incomplete subscription with old pending invoices",
		"subscription_id", sub.ID,
		"customer_id", sub.CustomerID,
		"current_status", sub.SubscriptionStatus)

	// Create cancellation request
	cancelRequest := dto.CancelSubscriptionRequest{
		CancellationType:  types.CancellationTypeImmediate,
		Reason:            "Automatic cancellation due to old pending invoices",
		ProrationBehavior: types.ProrationBehaviorNone, // No proration for failed payment scenarios
	}

	// Cancel the subscription
	_, err := h.subscriptionService.CancelSubscription(ctx, sub.ID, &cancelRequest)
	if err != nil {
		return err
	}

	h.logger.Infow("successfully cancelled incomplete subscription",
		"subscription_id", sub.ID,
		"customer_id", sub.CustomerID,
		"reason", "old_pending_invoices_cleanup")

	return nil
}

// voidOldPendingInvoice voids an old pending invoice in both FlexPrice and Stripe
func (h *InvoiceHandler) voidOldPendingInvoice(
	ctx context.Context,
	inv *dto.InvoiceResponse,
) error {

	// Void invoice in FlexPrice
	voidRequest := dto.InvoiceVoidRequest{
		Metadata: types.Metadata{
			"voided_by":   "cron_job",
			"void_reason": "old_pending_invoice_cleanup",
			"voided_at":   time.Now().UTC().Format(time.RFC3339),
		},
	}

	if err := h.invoiceService.VoidInvoice(ctx, inv.ID, voidRequest); err != nil {
		return err
	}

	// Void invoice in Stripe if it exists
	if err := h.voidInvoiceInStripe(ctx, inv.ID); err != nil {
		h.logger.Errorw("failed to void invoice in Stripe",
			"invoice_id", inv.ID,
			"error", err)
		// Don't fail the entire operation if Stripe void fails
		// The invoice is already voided in FlexPrice
	}

	return nil
}

// checkStripePartialPayment checks if an invoice has partial payments in Stripe
func (h *InvoiceHandler) checkStripePartialPayment(ctx context.Context, invoiceID string) (bool, error) {
	// Get Stripe invoice ID from entity integration mapping
	mapping, err := h.stripeService.GetExistingStripeMapping(ctx, invoiceID)
	if err != nil {
		// If no mapping exists, the invoice was never synced to Stripe
		h.logger.Debugw("no Stripe mapping found for invoice, assuming no partial payments", "invoice_id", invoiceID)
		return false, nil
	}

	stripeInvoiceID := mapping.ProviderEntityID

	// Get Stripe client
	stripeClient, err := h.stripeService.GetStripeClient(ctx)
	if err != nil {
		return false, err
	}

	// Retrieve the invoice from Stripe
	stripeInvoice, err := stripeClient.V1Invoices.Retrieve(ctx, stripeInvoiceID, nil)
	if err != nil {
		h.logger.Warnw("failed to retrieve invoice from Stripe",
			"invoice_id", invoiceID,
			"stripe_invoice_id", stripeInvoiceID,
			"error", err)
		// If we can't retrieve from Stripe, assume no partial payments to be safe
		return false, nil
	}

	// Check if there are any partial payments
	hasPartialPayment := stripeInvoice.AmountPaid > 0 && stripeInvoice.AmountPaid < stripeInvoice.Total

	if hasPartialPayment {
		h.logger.Infow("found partial payment in Stripe",
			"invoice_id", invoiceID,
			"stripe_invoice_id", stripeInvoiceID,
			"amount_paid", stripeInvoice.AmountPaid,
			"total", stripeInvoice.Total,
			"amount_remaining", stripeInvoice.AmountRemaining)
	}

	return hasPartialPayment, nil
}

// voidInvoiceInStripe voids an invoice in Stripe if it exists
func (h *InvoiceHandler) voidInvoiceInStripe(ctx context.Context, invoiceID string) error {
	// Get Stripe invoice ID from entity integration mapping
	mapping, err := h.stripeService.GetExistingStripeMapping(ctx, invoiceID)
	if err != nil {
		// If no mapping exists, the invoice was never synced to Stripe
		h.logger.Debugw("no Stripe mapping found for invoice", "invoice_id", invoiceID)
		return nil
	}

	stripeInvoiceID := mapping.ProviderEntityID
	h.logger.Infow("voiding invoice in Stripe",
		"invoice_id", invoiceID,
		"stripe_invoice_id", stripeInvoiceID)

	// Get Stripe client
	stripeClient, err := h.stripeService.GetStripeClient(ctx)
	if err != nil {
		return err
	}

	// Void the invoice in Stripe
	_, err = stripeClient.V1Invoices.VoidInvoice(ctx, stripeInvoiceID, &stripe.InvoiceVoidInvoiceParams{})
	if err != nil {
		return err
	}

	h.logger.Infow("successfully voided invoice in Stripe",
		"invoice_id", invoiceID,
		"stripe_invoice_id", stripeInvoiceID)

	return nil
}
