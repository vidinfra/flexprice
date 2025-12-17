package service

import (
	"context"
	"net/http"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/connection"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/security"
	"github.com/flexprice/flexprice/internal/types"
	"resty.dev/v3"
)

type SSLCommerzService struct {
	IPNURL        string
	StoreID       string
	StorePassword string
	BaseURL       string
	Client        *resty.Client
	ServiceParams
	encryptionService security.EncryptionService
}

func NewSSLCommerzService(params ServiceParams) *SSLCommerzService {
	encryptionService, err := security.NewEncryptionService(params.Config, params.Logger)
	if err != nil {
		params.Logger.Fatal("failed to initialize encryption service for SSLCommerz", "error", err)
	}

	return &SSLCommerzService{
		Client:            resty.New(),
		ServiceParams:     params,
		encryptionService: encryptionService,
	}
}

func (s *SSLCommerzService) decryptConnectionMetadata(encryptedSecretData types.ConnectionMetadata, providerType types.SecretProvider) (types.ConnectionMetadata, error) {
	decryptedData := encryptedSecretData

	switch providerType {
	case types.SecretProviderSSLCommerz:
		if encryptedSecretData.SSLCommerz != nil {
			decryptedStoreID, err := s.encryptionService.Decrypt(encryptedSecretData.SSLCommerz.StoreID)
			if err != nil {
				return types.ConnectionMetadata{}, err
			}
			decryptedStorePassword, err := s.encryptionService.Decrypt(encryptedSecretData.SSLCommerz.StorePassword)
			if err != nil {
				return types.ConnectionMetadata{}, err
			}
			decryptedData.SSLCommerz = &types.SSLCommerzConnectionMetadata{
				StoreID:       decryptedStoreID,
				StorePassword: decryptedStorePassword,
			}
		}
	default:
		if encryptedSecretData.Generic != nil {
			decryptedMap := make(map[string]interface{})
			for key, value := range encryptedSecretData.Generic.Data {
				if strValue, ok := value.(string); ok {
					decryptedValue, err := s.encryptionService.Decrypt(strValue)
					if err != nil {
						return types.ConnectionMetadata{}, err
					}
					decryptedMap[key] = decryptedValue
				} else {
					decryptedMap[key] = value
				}
			}
			decryptedData.Generic = &types.GenericConnectionMetadata{
				Data: decryptedMap,
			}
		}
	}

	return decryptedData, nil
}

func (s *SSLCommerzService) GetDecryptedSSLCommerzConfig(conn *connection.Connection) (*connection.SSLCommerzConnection, error) {
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

	return tempConn.GetSSLCommerzConfig()
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

	req.StoreID = s.StoreID
	req.StorePassword = s.StorePassword
	req.IPNURL = s.IPNURL
	req.Payment.SuccessURL = successURL
	req.Payment.FailURL = cancelURL
	req.Payment.CancelURL = cancelURL

	paymentURL := s.BaseURL + "/gwprocess/v4/api.php" //TODO: make it configurable later
	var response dto.SSLCommerzCreatePaymentLinkResponse
	resp, err := s.Client.R().
		SetFormData(map[string]string{
			"store_id":         req.StoreID,
			"store_passwd":     req.StorePassword,
			"total_amount":     req.Payment.TotalAmount.String(),
			"currency":         req.Payment.Currency,
			"tran_id":          req.Payment.TranID,
			"success_url":      req.Payment.SuccessURL,
			"fail_url":         req.Payment.FailURL,
			"cancel_url":       req.Payment.CancelURL,
			"cus_name":         req.Customer.Name,
			"cus_email":        req.Customer.Email,
			"cus_add1":         req.Customer.Add1,
			"cus_city":         req.Customer.City,
			"cus_postcode":     req.Customer.Postcode,
			"cus_country":      req.Customer.Country,
			"cus_phone":        req.Customer.Phone,
			"shipping_method":  req.ShippingMethod,
			"product_name":     req.Product.Name,
			"product_category": req.Product.Category,
			"product_profile":  req.Product.Profile,
		}).
		SetResult(&response).
		Post(paymentURL)

	if err != nil {
		return nil, ierr.NewError("failed to create payment link").
			WithHint("SSL Commerz API request failed").
			WithReportableDetails(map[string]interface{}{
				"invoice_id": req.InvoiceID,
			}).
			Mark(ierr.ErrInternal)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, ierr.NewError("failed to create payment link").
			WithHint("SSL Commerz API returned an error").
			WithReportableDetails(map[string]interface{}{
				"invoice_id": req.InvoiceID,
				"response":   response,
			}).
			Mark(ierr.ErrInternal)
	}

	return &response, nil
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
