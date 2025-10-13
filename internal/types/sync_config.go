package types

import (
	ierr "github.com/flexprice/flexprice/internal/errors"
)

// SyncInterval represents the sync interval for exports
type SyncInterval string

const (
	SyncIntervalDaily   SyncInterval = "daily"
	SyncIntervalWeekly  SyncInterval = "weekly"
	SyncIntervalMonthly SyncInterval = "monthly"
)

// Validate validates the sync interval
func (s SyncInterval) Validate() error {
	allowedIntervals := []SyncInterval{
		SyncIntervalDaily,
		SyncIntervalWeekly,
		SyncIntervalMonthly,
	}
	if s == "" {
		return nil // Optional field
	}
	for _, interval := range allowedIntervals {
		if s == interval {
			return nil
		}
	}
	return ierr.NewError("invalid sync interval").
		WithHint("Sync interval must be one of: daily, weekly, monthly").
		Mark(ierr.ErrValidation)
}

// ExportEntityType represents the entity type for exports
type ExportEntityType string

const (
	ExportEntityTypeFeatureUsage ExportEntityType = "feature_usage"
	ExportEntityTypeCustomer     ExportEntityType = "customer"
	ExportEntityTypeInvoice      ExportEntityType = "invoice"
	ExportEntityTypePrice        ExportEntityType = "price"
	ExportEntityTypeSubscription ExportEntityType = "subscription"
	ExportEntityTypeCreditNote   ExportEntityType = "credit_note"
)

// Validate validates the entity type
func (e ExportEntityType) Validate() error {
	allowedTypes := []ExportEntityType{
		ExportEntityTypeFeatureUsage,
		ExportEntityTypeCustomer,
		ExportEntityTypeInvoice,
		ExportEntityTypePrice,
		ExportEntityTypeSubscription,
		ExportEntityTypeCreditNote,
	}
	if e == "" {
		return nil // Optional field
	}
	for _, entityType := range allowedTypes {
		if e == entityType {
			return nil
		}
	}
	return ierr.NewError("invalid entity type").
		WithHint("Entity type must be one of: feature_usage, customer, invoice, price, subscription, credit_note").
		Mark(ierr.ErrValidation)
}

// S3ExportConfig represents S3 export configuration (non-sensitive settings)
// This goes in the sync_config column
type S3ExportConfig struct {
	Bucket           string             `json:"bucket"`                       // S3 bucket name
	Region           string             `json:"region"`                       // AWS region (e.g., "us-west-2")
	KeyPrefix        string             `json:"key_prefix,omitempty"`         // Optional prefix for S3 keys (e.g., "flexprice-exports/")
	Compression      string             `json:"compression,omitempty"`        // Compression type: "gzip", "none" (default: "gzip")
	Encryption       string             `json:"encryption,omitempty"`         // Encryption type: "AES256", "aws:kms" (default: "AES256")
	EndpointURL      string             `json:"endpoint_url,omitempty"`       // Custom endpoint URL for S3-compatible services
	VirtualHostStyle bool               `json:"virtual_host_style,omitempty"` // Use virtual-hosted-style URLs (default: false)
	MaxFileSizeMB    int                `json:"max_file_size_mb,omitempty"`   // Maximum file size in MB (default: 100)
	Interval         SyncInterval       `json:"interval,omitempty"`           // Sync interval: "daily", "weekly", "monthly" (default: "daily")
	EntityTypes      []ExportEntityType `json:"entity_types,omitempty"`       // Entity types to export (e.g., ["feature_usage", "customer"])
	SyncActive       bool               `json:"sync_active"`                  // Whether the sync is active (default: false)
}

// Validate validates the S3 export configuration
func (s *S3ExportConfig) Validate() error {
	if s == nil {
		return nil
	}

	if s.Bucket == "" {
		return ierr.NewError("bucket is required").
			WithHint("S3 bucket name is required").
			Mark(ierr.ErrValidation)
	}
	if s.Region == "" {
		return ierr.NewError("region is required").
			WithHint("AWS region is required").
			Mark(ierr.ErrValidation)
	}

	// Validate compression type if provided
	if s.Compression != "" && s.Compression != "gzip" && s.Compression != "none" {
		return ierr.NewError("invalid compression type").
			WithHint("Compression must be one of: gzip, none").
			Mark(ierr.ErrValidation)
	}

	// Validate encryption type if provided
	if s.Encryption != "" && s.Encryption != "AES256" && s.Encryption != "aws:kms" {
		return ierr.NewError("invalid encryption type").
			WithHint("Encryption must be one of: AES256, aws:kms").
			Mark(ierr.ErrValidation)
	}

	// Validate sync interval
	if err := s.Interval.Validate(); err != nil {
		return err
	}

	// Validate entity types
	for _, entityType := range s.EntityTypes {
		if err := entityType.Validate(); err != nil {
			return err
		}
	}

	return nil
}

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
