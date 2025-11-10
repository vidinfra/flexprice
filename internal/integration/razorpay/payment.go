package razorpay

import (
	"context"
	"fmt"
	"strings"

	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/interfaces"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// PaymentService handles Razorpay payment operations
type PaymentService struct {
	client      RazorpayClient
	customerSvc RazorpayCustomerService
	logger      *logger.Logger
}

// NewPaymentService creates a new Razorpay payment service
func NewPaymentService(
	client RazorpayClient,
	customerSvc RazorpayCustomerService,
	logger *logger.Logger,
) *PaymentService {
	return &PaymentService{
		client:      client,
		customerSvc: customerSvc,
		logger:      logger,
	}
}

// CreatePaymentLink creates a Razorpay payment link
func (s *PaymentService) CreatePaymentLink(ctx context.Context, req *CreatePaymentLinkRequest, customerService interfaces.CustomerService, invoiceService interfaces.InvoiceService) (*RazorpayPaymentLinkResponse, error) {
	s.logger.Infow("creating razorpay payment link",
		"invoice_id", req.InvoiceID,
		"customer_id", req.CustomerID,
		"amount", req.Amount.String(),
		"currency", req.Currency,
		"environment_id", req.EnvironmentID,
	)

	// Validate invoice and check payment eligibility
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

	// Ensure customer is synced to Razorpay before creating payment link
	flexpriceCustomer, err := s.customerSvc.EnsureCustomerSyncedToRazorpay(ctx, req.CustomerID, customerService)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to sync customer to Razorpay").
			WithReportableDetails(map[string]interface{}{
				"customer_id": req.CustomerID,
			}).
			Mark(ierr.ErrValidation)
	}

	// Get Razorpay customer ID (should exist after sync)
	razorpayCustomerID, exists := flexpriceCustomer.Metadata["razorpay_customer_id"]
	if !exists || razorpayCustomerID == "" {
		return nil, ierr.NewError("customer does not have Razorpay customer ID after sync").
			WithHint("Failed to sync customer to Razorpay").
			WithReportableDetails(map[string]interface{}{
				"customer_id": req.CustomerID,
			}).
			Mark(ierr.ErrValidation)
	}

	// Convert amount to smallest currency unit (paise for INR, cents for USD, etc.)
	// Razorpay expects amounts in smallest currency unit
	amountInSmallestUnit := req.Amount.Mul(decimal.NewFromInt(100)).IntPart()

	// Build description with invoice information
	var descriptionParts []string

	// Add invoice information
	invoiceInfo := fmt.Sprintf("Invoice: %s", lo.FromPtrOr(invoiceResp.InvoiceNumber, req.InvoiceID))
	descriptionParts = append(descriptionParts, invoiceInfo)

	// Add invoice total
	totalInfo := fmt.Sprintf("Total: %s %s", invoiceResp.Total.String(), invoiceResp.Currency)
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

			// Determine entity type and name
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
			itemDetail := fmt.Sprintf("%s: %s (%s %s)", entityType, itemName, lineItem.Amount.String(), invoiceResp.Currency)
			itemDetails = append(itemDetails, itemDetail)
		}

		if len(itemDetails) > 0 {
			descriptionParts = append(descriptionParts, itemDetails...)
		}
	}

	description := strings.Join(descriptionParts, " | ")

	// Build notes with metadata
	notes := map[string]interface{}{
		"flexprice_invoice_id":  req.InvoiceID,
		"flexprice_customer_id": req.CustomerID,
		"flexprice_payment_id":  req.PaymentID,
		"environment_id":        req.EnvironmentID,
		"payment_source":        "flexprice",
	}

	// Add custom metadata if provided
	for k, v := range req.Metadata {
		notes[k] = v
	}

	// Build description with line items details
	// Razorpay doesn't support line_items field in payment links, so we include them in description
	descriptionWithLineItems := description
	if len(invoiceResp.LineItems) > 0 {
		descriptionWithLineItems += " | "
		lineItemDescriptions := []string{}
		for _, item := range invoiceResp.LineItems {
			// Get display name with fallback
			itemName := "Line Item"
			if item.DisplayName != nil && *item.DisplayName != "" {
				itemName = *item.DisplayName
			} else if item.PlanDisplayName != nil && *item.PlanDisplayName != "" {
				itemName = *item.PlanDisplayName
			}

			// Format: "Item Name (Amount Currency)"
			itemDesc := fmt.Sprintf("%s (%s %s)",
				itemName,
				item.Amount.StringFixed(2),
				strings.ToUpper(item.Currency))
			lineItemDescriptions = append(lineItemDescriptions, itemDesc)
		}
		descriptionWithLineItems += strings.Join(lineItemDescriptions, " | ")

		s.logger.Infow("added line items to description",
			"invoice_id", req.InvoiceID,
			"line_items_count", len(invoiceResp.LineItems))
	}

	// Build customer info object
	// Razorpay payment links require customer object with name, email, and optionally contact
	customerInfo := map[string]interface{}{
		"name": flexpriceCustomer.Name,
	}
	if flexpriceCustomer.Email != "" {
		customerInfo["email"] = flexpriceCustomer.Email
	}
	// Note: contact/phone not available in FlexPrice customer model

	// Prepare payment link data according to Razorpay API format
	// Following the exact format from Razorpay documentation
	paymentLinkData := map[string]interface{}{
		"amount":      amountInSmallestUnit,
		"currency":    strings.ToUpper(req.Currency),
		"description": descriptionWithLineItems,
		"customer":    customerInfo,
		"notify": map[string]interface{}{
			"sms":   true,
			"email": true,
		},
		"reminder_enable": true,
		"notes":           notes,
	}

	// Add callback URL if provided (success URL)
	if req.SuccessURL != "" {
		paymentLinkData["callback_url"] = req.SuccessURL
		paymentLinkData["callback_method"] = "get" // Use GET method for callback
	}

	s.logger.Infow("creating payment link in Razorpay",
		"invoice_id", req.InvoiceID,
		"customer_id", req.CustomerID,
		"razorpay_customer_id", razorpayCustomerID,
		"customer_name", flexpriceCustomer.Name,
		"customer_email", flexpriceCustomer.Email,
		"amount", amountInSmallestUnit,
		"currency", req.Currency)

	// Create payment link in Razorpay using wrapper function
	razorpayPaymentLink, err := s.client.CreatePaymentLink(ctx, paymentLinkData)
	if err != nil {
		s.logger.Errorw("failed to create Razorpay payment link",
			"error", err,
			"invoice_id", req.InvoiceID)
		return nil, err
	}

	// Extract response fields
	paymentLinkID := razorpayPaymentLink["id"].(string)
	paymentLinkURL := razorpayPaymentLink["short_url"].(string)
	status := razorpayPaymentLink["status"].(string)
	createdAt := int64(razorpayPaymentLink["created_at"].(float64))

	response := &RazorpayPaymentLinkResponse{
		ID:         paymentLinkID,
		PaymentURL: paymentLinkURL,
		Amount:     req.Amount,
		Currency:   req.Currency,
		Status:     status,
		CreatedAt:  createdAt,
		PaymentID:  req.PaymentID,
	}

	s.logger.Infow("successfully created razorpay payment link",
		"payment_id", response.PaymentID,
		"payment_link_id", paymentLinkID,
		"payment_url", paymentLinkURL,
		"invoice_id", req.InvoiceID,
		"amount", req.Amount.String(),
		"currency", req.Currency,
	)

	return response, nil
}

// ReconcilePaymentWithInvoice updates the invoice payment status and amounts when a payment succeeds
func (s *PaymentService) ReconcilePaymentWithInvoice(ctx context.Context, paymentID string, paymentAmount decimal.Decimal, paymentService interfaces.PaymentService, invoiceService interfaces.InvoiceService) error {
	s.logger.Infow("starting payment reconciliation with invoice",
		"payment_id", paymentID,
		"payment_amount", paymentAmount.String())

	// Get the payment record
	payment, err := paymentService.GetPayment(ctx, paymentID)
	if err != nil {
		s.logger.Errorw("failed to get payment record for reconciliation",
			"error", err,
			"payment_id", paymentID)
		return err
	}

	s.logger.Infow("got payment record for reconciliation",
		"payment_id", paymentID,
		"invoice_id", payment.DestinationID,
		"payment_amount", payment.Amount.String())

	// Get the invoice
	invoiceResp, err := invoiceService.GetInvoice(ctx, payment.DestinationID)
	if err != nil {
		s.logger.Errorw("failed to get invoice for payment reconciliation",
			"error", err,
			"payment_id", paymentID,
			"invoice_id", payment.DestinationID)
		return err
	}

	s.logger.Infow("got invoice for reconciliation",
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

	s.logger.Infow("calculated new amounts for reconciliation",
		"payment_id", paymentID,
		"invoice_id", payment.DestinationID,
		"payment_amount", paymentAmount.String(),
		"new_amount_paid", newAmountPaid.String(),
		"new_amount_remaining", newAmountRemaining.String(),
		"new_payment_status", newPaymentStatus)

	// Update invoice payment status and amounts using reconciliation method
	s.logger.Infow("calling invoice reconciliation",
		"payment_id", paymentID,
		"invoice_id", payment.DestinationID,
		"payment_amount", paymentAmount.String(),
		"new_payment_status", newPaymentStatus)

	err = invoiceService.ReconcilePaymentStatus(ctx, payment.DestinationID, newPaymentStatus, &paymentAmount)
	if err != nil {
		s.logger.Errorw("failed to update invoice payment status during reconciliation",
			"error", err,
			"payment_id", paymentID,
			"invoice_id", payment.DestinationID,
			"payment_amount", paymentAmount.String(),
			"new_payment_status", newPaymentStatus)
		return err
	}

	s.logger.Infow("successfully reconciled payment with invoice",
		"payment_id", paymentID,
		"invoice_id", payment.DestinationID,
		"payment_amount", paymentAmount.String(),
		"new_payment_status", newPaymentStatus,
		"new_amount_paid", newAmountPaid.String(),
		"new_amount_remaining", newAmountRemaining.String())

	return nil
}
