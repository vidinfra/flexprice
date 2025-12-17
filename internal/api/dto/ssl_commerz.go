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
