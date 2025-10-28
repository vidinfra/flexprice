package models

import (
	ierr "github.com/flexprice/flexprice/internal/errors"
)

// HubSpotDealSyncWorkflowInput contains the input for the HubSpot deal sync workflow
type HubSpotDealSyncWorkflowInput struct {
	SubscriptionID string `json:"subscription_id"`
	TenantID       string `json:"tenant_id"`
	EnvironmentID  string `json:"environment_id"`
}

// Validate validates the workflow input
func (input *HubSpotDealSyncWorkflowInput) Validate() error {
	if input.SubscriptionID == "" {
		return ierr.NewError("subscription_id is required").
			WithHint("SubscriptionID must not be empty").
			Mark(ierr.ErrValidation)
	}
	if input.TenantID == "" {
		return ierr.NewError("tenant_id is required").
			WithHint("TenantID must not be empty").
			Mark(ierr.ErrValidation)
	}
	if input.EnvironmentID == "" {
		return ierr.NewError("environment_id is required").
			WithHint("EnvironmentID must not be empty").
			Mark(ierr.ErrValidation)
	}
	return nil
}
