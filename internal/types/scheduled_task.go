package types

import (
	ierr "github.com/flexprice/flexprice/internal/errors"
)

// ScheduledTaskInterval represents the interval for scheduled tasks
type ScheduledTaskInterval string

const (
	ScheduledTaskIntervalTesting ScheduledTaskInterval = "testing" // 10 minutes for testing
	ScheduledTaskIntervalHourly  ScheduledTaskInterval = "hourly"
	ScheduledTaskIntervalDaily   ScheduledTaskInterval = "daily"
	ScheduledTaskIntervalWeekly  ScheduledTaskInterval = "weekly"
	ScheduledTaskIntervalMonthly ScheduledTaskInterval = "monthly"
	ScheduledTaskIntervalYearly  ScheduledTaskInterval = "yearly"
)

// Validate validates the scheduled task interval
func (s ScheduledTaskInterval) Validate() error {
	allowedIntervals := []ScheduledTaskInterval{
		ScheduledTaskIntervalTesting,
		ScheduledTaskIntervalHourly,
		ScheduledTaskIntervalDaily,
		ScheduledTaskIntervalWeekly,
		ScheduledTaskIntervalMonthly,
		ScheduledTaskIntervalYearly,
	}
	if s == "" {
		return ierr.NewError("interval is required").
			WithHint("Scheduled task interval must be specified").
			Mark(ierr.ErrValidation)
	}
	for _, interval := range allowedIntervals {
		if s == interval {
			return nil
		}
	}
	return ierr.NewError("invalid scheduled task interval").
		WithHint("Interval must be one of: testing, hourly, daily, weekly, monthly, yearly").
		Mark(ierr.ErrValidation)
}

// ScheduledTaskEntityType represents the entity type for scheduled tasks
type ScheduledTaskEntityType string

const (
	ScheduledTaskEntityTypeEvents       ScheduledTaskEntityType = "events"
	ScheduledTaskEntityTypeCustomer     ScheduledTaskEntityType = "customer"
	ScheduledTaskEntityTypeInvoice      ScheduledTaskEntityType = "invoice"
	ScheduledTaskEntityTypePrice        ScheduledTaskEntityType = "price"
	ScheduledTaskEntityTypeSubscription ScheduledTaskEntityType = "subscription"
	ScheduledTaskEntityTypeCreditNote   ScheduledTaskEntityType = "credit_note"
)

// Validate validates the entity type
func (e ScheduledTaskEntityType) Validate() error {
	allowedTypes := []ScheduledTaskEntityType{
		ScheduledTaskEntityTypeEvents,
		ScheduledTaskEntityTypeCustomer,
		ScheduledTaskEntityTypeInvoice,
		ScheduledTaskEntityTypePrice,
		ScheduledTaskEntityTypeSubscription,
		ScheduledTaskEntityTypeCreditNote,
	}
	if e == "" {
		return ierr.NewError("entity type is required").
			WithHint("Scheduled task entity type must be specified").
			Mark(ierr.ErrValidation)
	}
	for _, entityType := range allowedTypes {
		if e == entityType {
			return nil
		}
	}
	return ierr.NewError("invalid entity type").
		WithHint("Entity type must be one of: events, customer, invoice, price, subscription, credit_note").
		Mark(ierr.ErrValidation)
}

// ScheduledTaskStatus represents the status of a scheduled task run
type ScheduledTaskStatus string

const (
	ScheduledTaskStatusSuccess ScheduledTaskStatus = "success"
	ScheduledTaskStatusFailed  ScheduledTaskStatus = "failed"
	ScheduledTaskStatusRunning ScheduledTaskStatus = "running"
)

// S3JobConfig represents the configuration for an S3 export job
// This is stored in the job_config JSON field of scheduled_tasks table
type S3JobConfig struct {
	Bucket           string `json:"bucket"`                       // S3 bucket name
	Region           string `json:"region"`                       // AWS region (e.g., "us-west-2")
	KeyPrefix        string `json:"key_prefix,omitempty"`         // Optional prefix for S3 keys (e.g., "flexprice-exports/")
	Compression      string `json:"compression,omitempty"`        // Compression type: "gzip", "none" (default: "gzip")
	Encryption       string `json:"encryption,omitempty"`         // Encryption type: "AES256", "aws:kms" (default: "AES256")
	EndpointURL      string `json:"endpoint_url,omitempty"`       // Custom endpoint URL for S3-compatible services
	VirtualHostStyle bool   `json:"virtual_host_style,omitempty"` // Use virtual-hosted-style URLs (default: false)
	MaxFileSizeMB    int    `json:"max_file_size_mb,omitempty"`   // Maximum file size in MB (default: 100)
}

// Validate validates the S3 job configuration
func (s *S3JobConfig) Validate() error {
	if s == nil {
		return ierr.NewError("S3 job config is required").
			WithHint("S3 job configuration must be provided").
			Mark(ierr.ErrValidation)
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

	// Set defaults
	if s.Compression == "" {
		s.Compression = "gzip"
	}
	if s.Encryption == "" {
		s.Encryption = "AES256"
	}
	if s.MaxFileSizeMB == 0 {
		s.MaxFileSizeMB = 100
	}

	return nil
}

// SetDefaults sets default values for the S3 job configuration
func (s *S3JobConfig) SetDefaults() {
	if s.Compression == "" {
		s.Compression = "gzip"
	}
	if s.Encryption == "" {
		s.Encryption = "AES256"
	}
	if s.MaxFileSizeMB == 0 {
		s.MaxFileSizeMB = 100
	}
}

// CreateScheduledTaskInput represents the input for creating a scheduled task
type CreateScheduledTaskInput struct {
	ConnectionID string
	EntityType   ScheduledTaskEntityType
	Interval     ScheduledTaskInterval
	Enabled      bool
	JobConfig    map[string]interface{}
}

// UpdateScheduledTaskInput represents the input for updating a scheduled task
type UpdateScheduledTaskInput struct {
	Interval  *ScheduledTaskInterval
	Enabled   *bool
	JobConfig *map[string]interface{}
}
