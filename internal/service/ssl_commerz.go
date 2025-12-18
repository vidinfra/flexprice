package service

import (
	"context"
	"net/http"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/connection"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/security"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/k0kubun/pp"
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
		pp.Println("Decrypting SSL Commerz connection metadata. Case SSLCommerz")
		if encryptedSecretData.SSLCommerz != nil {
			decryptedStoreID, err := s.encryptionService.Decrypt(encryptedSecretData.SSLCommerz.StoreID)
			if err != nil {
				return types.ConnectionMetadata{}, err
			}
			decryptedStorePassword, err := s.encryptionService.Decrypt(encryptedSecretData.SSLCommerz.StorePassword)
			pp.Println("Decrypting SSL Commerz connection metadata. StorePassword decryption result: ", err)
			if err != nil {
				pp.Println("Failed to decrypt SSL Commerz StorePassword: ", err)
				return types.ConnectionMetadata{}, err
			}
			pp.Println("Successfully decrypted SSL Commerz Config. ", decryptedStoreID, decryptedStorePassword)
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

	// Get SSLCommerz connection for this environment
	conn, err := s.ConnectionRepo.GetByProvider(ctx, types.SecretProviderSSLCommerz)
	if err != nil {
		return nil, ierr.NewError("failed to get SSL Commerz connection").
			WithHint("SSL Commerz connection not configured for this environment").
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

	// Initialize SSLCommerz configuration
	sslCommerzConfig, err := s.GetDecryptedSSLCommerzConfig(conn)
	if err != nil {
		return nil, ierr.NewError("failed to get SSL Commerz configuration").
			WithHint("Invalid SSL Commerz configuration").
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

	req.StoreID = sslCommerzConfig.StoreID
	req.StorePassword = "tb69426fab42fcf@ssl" // sslCommerzConfig.StorePassword
	req.IPNURL = s.Config.SSLCommerz.IpnURL
	req.Payment.SuccessURL = successURL
	req.Payment.FailURL = cancelURL
	req.Payment.CancelURL = cancelURL

	pp.Println("Final SSL Commerz Request: ", sslCommerzConfig)

	paymentURL := s.Config.SSLCommerz.BaseURL + "/gwprocess/v4/api.php"
	var response dto.SSLCommerzCreatePaymentLinkResponse

	form := dto.SSLCommerzFormData{
		StoreID:         req.StoreID,
		StorePassword:   req.StorePassword,
		TotalAmount:     req.Payment.TotalAmount.String(),
		Currency:        req.Payment.Currency,
		TranID:          req.InvoiceID,
		SuccessURL:      req.Payment.SuccessURL,
		FailURL:         req.Payment.FailURL,
		CancelURL:       req.Payment.CancelURL,
		CusName:         req.Customer.Name,
		CusEmail:        req.CustomerID,
		CusAdd1:         req.Customer.Add1,
		CusCity:         req.Customer.City,
		CusPostcode:     req.Customer.Postcode,
		CusCountry:      req.Customer.Country,
		CusPhone:        req.CustomerID,
		ShippingMethod:  "NO",
		ProductName:     "Tenbyte Service Payment",
		ProductCategory: "Service",
		ProductProfile:  "General",
	}

	resp, err := s.Client.R().
		SetFormData(form.ToMap()).
		SetResult(&response).
		Post(paymentURL)

	pp.Println("SSL Commerz API Response: ", response)

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
