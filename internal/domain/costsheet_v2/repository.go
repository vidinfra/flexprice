/*
Package costsheet_v2 provides repository interfaces for managing costsheet version 2 data access.
This package defines the contract for data persistence operations on costsheet v2 entities.
*/
package costsheet_v2

import (
	"context"

	"github.com/flexprice/flexprice/internal/types"
)

// Repository defines the contract for costsheet v2 data access operations.
// It provides methods for CRUD operations and querying costsheet v2 records.
type Repository interface {
	// Create creates a new costsheet v2 record
	Create(ctx context.Context, costsheet *CostsheetV2) error

	// GetByID retrieves a costsheet v2 record by its ID
	GetByID(ctx context.Context, id string) (*CostsheetV2, error)

	// GetByTenantAndEnvironment retrieves costsheet v2 records for a specific tenant and environment
	GetByTenantAndEnvironment(ctx context.Context, tenantID, environmentID string) ([]*CostsheetV2, error)

	// List retrieves costsheet v2 records based on the provided filter
	List(ctx context.Context, filter *types.CostsheetV2Filter) ([]*CostsheetV2, error)

	// Count returns the total number of costsheet v2 records matching the filter
	Count(ctx context.Context, filter *types.CostsheetV2Filter) (int64, error)

	// Update updates an existing costsheet v2 record
	Update(ctx context.Context, costsheet *CostsheetV2) error

	// Delete soft deletes a costsheet v2 record by setting its status to deleted
	Delete(ctx context.Context, id string) error

	// GetByName retrieves a costsheet v2 record by name within a tenant and environment
	GetByName(ctx context.Context, tenantID, environmentID, name string) (*CostsheetV2, error)

	// ListByStatus retrieves costsheet v2 records filtered by status
	ListByStatus(ctx context.Context, tenantID, environmentID string, status types.Status) ([]*CostsheetV2, error)
}
