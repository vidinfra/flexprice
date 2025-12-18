package dto

import (
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

type CreateSSLPaymentLinkRequest struct {
	InvoiceID              string          `json:"invoice_id" binding:"required"`
	CustomerID             string          `json:"customer_id" binding:"required"`
	Amount                 decimal.Decimal `json:"amount" binding:"required"`
	EnvironmentID          string          `json:"environment_id" binding:"required"`
	Currency               string          `json:"currency" binding:"required"`
	SuccessURL             string          `json:"success_url,omitempty"`
	CancelURL              string          `json:"cancel_url,omitempty"`
	Metadata               types.Metadata  `json:"metadata,omitempty"`
	SaveCardAndMakeDefault bool            `json:"save_card_and_make_default" default:"false"`

	StoreID       string `json:"store_id"`
	StorePassword string `json:"store_passwd"`
	IPNURL        string `json:"ipn_url"`

	ShippingMethod string `json:"shipping_method"`
	Payment
	Product
	Customer
	CustomMeta
}

type Payment struct {
	TranID      string          `json:"tran_id,omitempty"`
	TotalAmount decimal.Decimal `json:"total_amount"`
	Currency    string          `json:"currency"`

	SuccessURL string `json:"success_url"`
	FailURL    string `json:"fail_url"`
	CancelURL  string `json:"cancel_url"`
}

type Product struct {
	Name           string          `json:"product_name"`
	Category       string          `json:"product_category"`
	Profile        string          `json:"product_profile"`
	Amount         decimal.Decimal `json:"product_amount,omitempty,omitzero"`
	Vat            decimal.Decimal `json:"vat,omitzero"`
	DiscountAmount decimal.Decimal `json:"discount_amount,omitzero"`
	ConvenienceFee decimal.Decimal `json:"convenience_fee,omitzero"`
}

type Customer struct {
	Name     string `json:"cus_name"`
	Email    string `json:"cus_email"`
	Add1     string `json:"cus_add1"`
	Add2     string `json:"cus_add2,omitempty"`
	City     string `json:"cus_city"`
	State    string `json:"cus_state,omitempty"`
	Postcode string `json:"cus_postcode"`
	Phone    string `json:"cus_phone"`
	Country  string `json:"cus_country"`
}

type CustomMeta struct {
	ValueA string `json:"value_a,omitempty" form:"value_a,omitempty"`
	ValueB string `json:"value_b,omitempty" form:"value_b,omitempty"`
	ValueC string `json:"value_c,omitempty" form:"value_c,omitempty"`
	ValueD string `json:"value_d,omitempty" form:"value_d,omitempty"`
}

type SSLCommerzCreatePaymentLinkResponse struct {
	ID                 string          `json:"id"`
	PaymentURL         string          `json:"payment_url"`
	PaymentIntentID    string          `json:"payment_intent_id"`
	Amount             decimal.Decimal `json:"amount"`
	Currency           string          `json:"currency"`
	Status             string          `json:"status"`
	CreatedAt          int64           `json:"created_at"`
	PaymentID          string          `json:"payment_id,omitempty"`
	FailedReason       string          `json:"failedreason"`
	SessionKey         string          `json:"sessionkey"`
	GatewayPageURL     string          `json:"GatewayPageURL"`
	StoreBanner        string          `json:"storeBanner"`
	StoreLogo          string          `json:"storeLogo"`
	RedirectGatewayURL string          `json:"redirectGatewayURL"`
}

type SSLCommerzValidationResponse struct {
	Status                string `json:"status"`
	TranDate              string `json:"tran_date"`
	TranID                string `json:"tran_id"`
	ValID                 string `json:"val_id"`
	Amount                string `json:"amount"`
	StoreAmount           string `json:"store_amount"`
	Currency              string `json:"currency"`
	BankTranID            string `json:"bank_tran_id"`
	CardType              string `json:"card_type"`
	CardNo                string `json:"card_no"`
	CardIssuer            string `json:"card_issuer"`
	CardBrand             string `json:"card_brand"`
	CardIssuerCountry     string `json:"card_issuer_country"`
	CardIssuerCountryCode string `json:"card_issuer_country_code"`
	CurrencyType          string `json:"currency_type"`
	CurrencyAmount        string `json:"currency_amount"`
	CurrencyRate          string `json:"currency_rate"`
	BaseFair              string `json:"base_fair"`
	ValueA                string `json:"value_a"`
	ValueB                string `json:"value_b"`
	ValueC                string `json:"value_c"`
	ValueD                string `json:"value_d"`
	EmiInstalment         string `json:"emi_instalment"`
	EmiAmount             string `json:"emi_amount"`
	EmiDescription        string `json:"emi_description"`
	EmiIssuer             string `json:"emi_issuer"`
	AccountDetails        string `json:"account_details"`
	RiskTitle             string `json:"risk_title"`
	RiskLevel             string `json:"risk_level"`
	APIConnect            string `json:"APIConnect"`
	ValidatedOn           string `json:"validated_on"`
	GwVersion             string `json:"gw_version"`
}

type SSLCommerzFormData struct {
	StoreID         string
	StorePassword   string
	ValueA          string
	ValueB          string
	TotalAmount     string
	Currency        string
	TranID          string
	SuccessURL      string
	FailURL         string
	CancelURL       string
	CusName         string
	CusEmail        string
	CusAdd1         string
	CusCity         string
	CusPostcode     string
	CusCountry      string
	CusPhone        string
	ShippingMethod  string
	ProductName     string
	ProductCategory string
	ProductProfile  string
}

func (f *SSLCommerzFormData) ToMap() map[string]string {
	return map[string]string{
		"store_id":         f.StoreID,
		"store_passwd":     f.StorePassword,
		"total_amount":     f.TotalAmount,
		"value_a":          f.ValueA,
		"value_b":          f.ValueB,
		"currency":         f.Currency,
		"tran_id":          f.TranID,
		"success_url":      f.SuccessURL,
		"fail_url":         f.FailURL,
		"cancel_url":       f.CancelURL,
		"cus_name":         f.CusName,
		"cus_email":        f.CusEmail,
		"cus_add1":         f.CusAdd1,
		"cus_city":         f.CusCity,
		"cus_postcode":     f.CusPostcode,
		"cus_country":      f.CusCountry,
		"cus_phone":        f.CusPhone,
		"shipping_method":  f.ShippingMethod,
		"product_name":     f.ProductName,
		"product_category": f.ProductCategory,
		"product_profile":  f.ProductProfile,
	}
}
