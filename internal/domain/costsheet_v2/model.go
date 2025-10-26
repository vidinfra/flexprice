/*
Package costsheet_v2 provides domain models and operations for managing costsheet version 2 in the FlexPrice system.
CostsheetV2 is used for tracking cost-related configurations with basic columns and is designed for comparing
revenue and costsheet calculations.
*/
package costsheet_v2

import (
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
)

// CostsheetV2 represents the domain model for costsheet version 2.
// It includes basic columns as specified in the requirements:
// - id, name, tenant_id, environment_id, status, created_at, created_by, updated_at, updated_by
// - lookup_key, description, metadata for additional information
// This entity is used for comparing revenue and costsheet calculations.
type CostsheetV2 struct {
	// ID uniquely identifies this costsheet v2 record
	ID string `json:"id"`

	// Name of the costsheet
	Name string `json:"name"`

	// LookupKey for easy identification and retrieval
	LookupKey string `json:"lookup_key,omitempty"`

	// Description provides additional context about the costsheet
	Description string `json:"description,omitempty"`

	// Metadata stores additional key-value pairs for extensibility
	Metadata map[string]string `json:"metadata,omitempty"`

	// EnvironmentID for environment segregation
	EnvironmentID string `json:"environment_id"`

	// Embed BaseModel for common fields (tenant_id, status, timestamps, etc.)
	types.BaseModel
}

// Validate checks if the costsheet v2 data is valid.
// This includes checking required fields and valid status values.
func (c *CostsheetV2) Validate() error {
	if c.Name == "" {
		return ierr.NewError("name is required").
			WithHint("Costsheet name is required").
			Mark(ierr.ErrValidation)
	}

	// Validate status
	validStatuses := []types.Status{
		types.StatusPublished,
		types.StatusArchived,
		types.StatusDeleted,
	}
	isValidStatus := false
	for _, status := range validStatuses {
		if c.Status == status {
			isValidStatus = true
			break
		}
	}
	if !isValidStatus {
		return ierr.NewError("invalid status").
			WithHint("Status must be one of: published, archived, deleted").
			WithReportableDetails(map[string]any{
				"status":         c.Status,
				"valid_statuses": validStatuses,
			}).
			Mark(ierr.ErrValidation)
	}

	return nil
}
