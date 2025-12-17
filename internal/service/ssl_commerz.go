package service

import (
	"context"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/security"
	"github.com/flexprice/flexprice/internal/types"
)

type SSLCommerzService struct {
	ServiceParams
	encryptionService security.EncryptionService
}

func NewSSLCommerzService(params ServiceParams) *SSLCommerzService {
	encryptionService, err := security.NewEncryptionService(params.Config, params.Logger)
	if err != nil {
		params.Logger.Fatal("failed to initialize encryption service for SSLCommerz", "error", err)
	}

	return &SSLCommerzService{
		ServiceParams:     params,
		encryptionService: encryptionService,
	}
}

// CreatePaymentLink creates a SSL Commerz payment link
func (s *SSLCommerzService) CreatePaymentLink(ctx context.Context, req *dto.CreateSSLPaymentLinkRequest) (*dto.SSLCommerzCreatePaymentLinkResponse, error) {
	s.Logger.Infow("Creating SSL Commerz payment link",
		"invoice_id", req.InvoiceID,
		"amount", req.TotalAmount.String(),
		"currency", req.Currency,
		"customer_name", req.Customer.Name,
		"customer_email", req.Customer.Email,
	)

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

	successURL := req.SuccessURL
	if successURL == "" {
		successURL = "https://admin-dev.flexprice.io/customer-management/invoices?page=1"
	}

	cancelURL := req.CancelURL
	if cancelURL == "" {
		cancelURL = "https://admin-dev.flexprice.io/customer-management/invoices?page=1"
	}

	return nil, nil
}

func (s *SSLCommerzService) GetCustomerPaymentMethods() {}

func (s *SSLCommerzService) SetDefaultPaymentMethod() {}

func (s *SSLCommerzService) DetachPaymentMethod() {}

func (s *SSLCommerzService) GetPaymentMethodDetails() {}

func (s *SSLCommerzService) GetDefaultPaymentMethod() {}

func (s *SSLCommerzService) ChargeSavedPaymentMethod() {}

func (s *SSLCommerzService) HasSavedPaymentMethods() {}

func (s *SSLCommerzService) ParseWebhookEvent() {}

func (s *SSLCommerzService) VerifyWebhookSignature() {}

func (s *SSLCommerzService) GetPaymentStatus() {}

func (s *SSLCommerzService) GetPaymentStatusByPaymentIntent() {}

func (s *SSLCommerzService) AttachPaymentToSSLInvoice() {}

func (s *SSLCommerzService) GetPaymentIntent() {}

func (s *SSLCommerzService) SetupIntent() {}

func (s *SSLCommerzService) ListCustomerPaymentMethods() {}

func (s *SSLCommerzService) IsInvoiceSyncedToStripe() {}
