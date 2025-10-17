package types

import (
	ierr "github.com/flexprice/flexprice/internal/errors"
)

// SyncConfig defines which entities should be synced between FlexPrice and external providers
type SyncConfig struct {
	// Integration sync (Stripe, Razorpay, etc.)
	Plan         *EntitySyncConfig `json:"plan,omitempty"`
	Subscription *EntitySyncConfig `json:"subscription,omitempty"`
	Invoice      *EntitySyncConfig `json:"invoice,omitempty"`

	// Export sync (S3, Athena, BigQuery, etc.)
	FeatureUsage *EntitySyncConfig `json:"feature_usage,omitempty"`
	Customer     *EntitySyncConfig `json:"customer,omitempty"`
	Price        *EntitySyncConfig `json:"price,omitempty"`
	CreditNote   *EntitySyncConfig `json:"credit_note,omitempty"`

	// S3 Export Configuration (stored in sync_config column)
	S3 *S3ExportConfig `json:"s3,omitempty"`
}

// EntitySyncConfig defines sync direction for an entity
type EntitySyncConfig struct {
	Inbound  bool `json:"inbound"`  // Inbound from external provider to FlexPrice
	Outbound bool `json:"outbound"` // Outbound from FlexPrice to external provider
}

// DefaultSyncConfig returns a sync config with all entities disabled
func DefaultSyncConfig() *SyncConfig {
	return &SyncConfig{
		// Integration sync
		Plan:         &EntitySyncConfig{Inbound: false, Outbound: false},
		Subscription: &EntitySyncConfig{Inbound: false, Outbound: false},
		Invoice:      &EntitySyncConfig{Inbound: false, Outbound: false},

		// Export sync
		FeatureUsage: &EntitySyncConfig{Inbound: false, Outbound: false},
		Customer:     &EntitySyncConfig{Inbound: false, Outbound: false},
		Price:        &EntitySyncConfig{Inbound: false, Outbound: false},
		CreditNote:   &EntitySyncConfig{Inbound: false, Outbound: false},
	}
}

// Validate validates the SyncConfig
func (s *SyncConfig) Validate() error {
	if s == nil {
		return nil
	}

	if s.Plan != nil && s.Plan.Outbound {
		return ierr.NewError("plan outbound sync is not allowed").Mark(ierr.ErrValidation)
	}

	if s.Subscription != nil && s.Subscription.Outbound {
		return ierr.NewError("subscription outbound sync is not allowed").Mark(ierr.ErrValidation)
	}

	if s.Invoice != nil && s.Invoice.Inbound {
		return ierr.NewError("invoice inbound sync is not allowed").Mark(ierr.ErrValidation)
	}

	return nil
}
