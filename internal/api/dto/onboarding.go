package dto

import (
	"time"

	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/validator"
)

// OnboardingEventsRequest represents the request to generate events for onboarding
type OnboardingEventsRequest struct {
	CustomerID     string `json:"customer_id" validate:"omitempty" example:"cus_01HKG8QWERTY123"`
	FeatureID      string `json:"feature_id" validate:"omitempty" example:"feat_01HKG8QWERTY123"`
	SubscriptionID string `json:"subscription_id" validate:"omitempty" example:"sub_01HKG8QWERTY123"`
	Duration       int    `json:"duration" validate:"omitempty,min=1,max=600" default:"60" example:"60"`
}

// Validate validates the OnboardingEventsRequest
func (r *OnboardingEventsRequest) Validate() error {
	// Either customer_id + feature_id OR subscription_id must be provided
	if (r.CustomerID == "" || r.FeatureID == "") && r.SubscriptionID == "" {
		return ierr.NewError("either customer_id + feature_id or subscription_id must be provided").
			WithHint("Please provide Customer or Subscription information").
			Mark(ierr.ErrValidation)
	}

	// If both are provided, subscription_id takes precedence (we can add a warning log)
	return validator.ValidateRequest(r)
}

// OnboardingEventsResponse represents the response for onboarding events generation
type OnboardingEventsResponse struct {
	Message        string    `json:"message"`
	StartedAt      time.Time `json:"started_at"`
	Duration       int       `json:"duration"`
	Count          int       `json:"count"`
	CustomerID     string    `json:"customer_id"`
	FeatureID      string    `json:"feature_id,omitempty"`
	SubscriptionID string    `json:"subscription_id,omitempty"`
}

// SendEmailRequest represents a request to send a plain text email
// SendEmailRequest - plain text email for testing
// from_address is always read from config
type SendEmailRequest struct {
	ToAddress string `json:"to_address" validate:"required,email" example:"user@example.com"`
	Subject   string `json:"subject" validate:"required" example:"Welcome to Flexprice"`
	Text      string `json:"text" validate:"required" example:"Hello, welcome to our platform!"`
}

// Validate validates the SendEmailRequest
func (r *SendEmailRequest) Validate() error {
	return validator.ValidateRequest(r)
}

// SendEmailResponse represents the response from sending an email
type SendEmailResponse struct {
	Success   bool   `json:"success"`
	MessageID string `json:"message_id,omitempty"`
	Message   string `json:"message"`
	Error     string `json:"error,omitempty"`
}

// SendEmailWithTemplateRequest represents a request to send a templated email
// SendEmailWithTemplateRequest - minimal request for testing email templates
// from_address is always read from config, subject is determined by template
type SendEmailWithTemplateRequest struct {
	ToAddress string `json:"to_address" validate:"required,email" example:"user@example.com"`
	Template  string `json:"template" validate:"required" example:"welcome"` // "welcome" maps to welcome-email.html
}

// Validate validates the SendEmailWithTemplateRequest
func (r *SendEmailWithTemplateRequest) Validate() error {
	return validator.ValidateRequest(r)
}

// SendEmailWithTemplateResponse represents the response from sending a templated email
type SendEmailWithTemplateResponse struct {
	Success   bool   `json:"success"`
	MessageID string `json:"message_id,omitempty"`
	Message   string `json:"message"`
	Error     string `json:"error,omitempty"`
}
