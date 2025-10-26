// Package ent provides the implementation of the costsheet v2 repository using Ent ORM.
// This package handles all database operations related to costsheet v2 records, including CRUD operations
// and specialized queries.
package ent

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/ent/costsheetv2"
	"github.com/flexprice/flexprice/internal/cache"
	domainCostsheetV2 "github.com/flexprice/flexprice/internal/domain/costsheet_v2"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/types"
)

// costsheetV2Repository implements the costsheet_v2.Repository interface using Ent ORM.
// It provides methods for managing costsheet v2 records in the database.
type costsheetV2Repository struct {
	client postgres.IClient // Database client for executing queries
	logger *logger.Logger   // Logger for tracking operations
	cache  cache.Cache      // Cache for performance optimization
}

// NewCostSheetV2Repository creates a new instance of the Ent-based costsheet v2 repository.
// It initializes the repository with the provided database client, logger, and cache.
func NewCostSheetV2Repository(client postgres.IClient, logger *logger.Logger, cache cache.Cache) domainCostsheetV2.Repository {
	return &costsheetV2Repository{
		client: client,
		logger: logger,
		cache:  cache,
	}
}

// Create stores a new costsheet v2 record in the database.
func (r *costsheetV2Repository) Create(ctx context.Context, costsheet *domainCostsheetV2.CostsheetV2) error {
	entCostsheet := r.domainToEnt(costsheet)

	createQuery := r.client.Writer(ctx).CostsheetV2.Create().
		SetID(entCostsheet.ID).
		SetTenantID(entCostsheet.TenantID).
		SetEnvironmentID(entCostsheet.EnvironmentID).
		SetStatus(entCostsheet.Status).
		SetCreatedAt(entCostsheet.CreatedAt).
		SetUpdatedAt(entCostsheet.UpdatedAt).
		SetNillableCreatedBy(&entCostsheet.CreatedBy).
		SetNillableUpdatedBy(&entCostsheet.UpdatedBy).
		SetName(entCostsheet.Name)

	// Set optional fields
	if entCostsheet.LookupKey != "" {
		createQuery.SetLookupKey(entCostsheet.LookupKey)
	}
	if entCostsheet.Description != "" {
		createQuery.SetDescription(entCostsheet.Description)
	}
	if entCostsheet.Metadata != nil {
		createQuery.SetMetadata(entCostsheet.Metadata)
	}

	err := createQuery.Exec(ctx)

	if err != nil {
		r.logger.Errorw("Failed to create costsheet v2", "error", err, "costsheet_id", costsheet.ID)
		return ierr.WithError(err).
			WithHint("Failed to create costsheet v2").
			Mark(ierr.ErrDatabase)
	}

	r.logger.Infow("Created costsheet v2", "costsheet_id", costsheet.ID, "name", costsheet.Name)
	return nil
}

// GetByID retrieves a costsheet v2 record by its ID.
func (r *costsheetV2Repository) GetByID(ctx context.Context, id string) (*domainCostsheetV2.CostsheetV2, error) {
	entCostsheet, err := r.client.Reader(ctx).CostsheetV2.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ierr.NewError("costsheet v2 not found").
				WithHint("No costsheet v2 found with the provided ID").
				WithReportableDetails(map[string]any{"id": id}).
				Mark(ierr.ErrNotFound)
		}
		r.logger.Errorw("Failed to get costsheet v2 by ID", "error", err, "costsheet_id", id)
		return nil, ierr.WithError(err).
			WithHint("Failed to retrieve costsheet v2").
			Mark(ierr.ErrDatabase)
	}

	return r.entToDomain(entCostsheet), nil
}

// GetByTenantAndEnvironment retrieves costsheet v2 records for a specific tenant and environment.
func (r *costsheetV2Repository) GetByTenantAndEnvironment(ctx context.Context, tenantID, environmentID string) ([]*domainCostsheetV2.CostsheetV2, error) {
	entCostsheets, err := r.client.Reader(ctx).CostsheetV2.Query().
		Where(
			costsheetv2.TenantID(tenantID),
			costsheetv2.EnvironmentID(environmentID),
			costsheetv2.Status(string(types.StatusPublished)),
		).
		Order(ent.Asc(costsheetv2.FieldCreatedAt)).
		All(ctx)

	if err != nil {
		r.logger.Errorw("Failed to get costsheet v2 records by tenant and environment",
			"error", err, "tenant_id", tenantID, "environment_id", environmentID)
		return nil, ierr.WithError(err).
			WithHint("Failed to retrieve costsheet v2 records").
			Mark(ierr.ErrDatabase)
	}

	costsheets := make([]*domainCostsheetV2.CostsheetV2, len(entCostsheets))
	for i, entCostsheet := range entCostsheets {
		costsheets[i] = r.entToDomain(entCostsheet)
	}

	return costsheets, nil
}

// List retrieves costsheet v2 records based on the provided filter.
func (r *costsheetV2Repository) List(ctx context.Context, filter *types.CostsheetV2Filter) ([]*domainCostsheetV2.CostsheetV2, error) {
	query := r.client.Reader(ctx).CostsheetV2.Query()

	// Apply tenant and environment from context (like plan repository)
	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)
	if tenantID != "" {
		query = query.Where(costsheetv2.TenantID(tenantID))
	}
	if environmentID != "" {
		query = query.Where(costsheetv2.EnvironmentID(environmentID))
	}

	// Apply status filter if provided
	if filter.GetStatus() != "" {
		query = query.Where(costsheetv2.Status(filter.GetStatus()))
	}

	// Apply costsheet v2 IDs filter
	if len(filter.CostsheetV2IDs) > 0 {
		query = query.Where(costsheetv2.IDIn(filter.CostsheetV2IDs...))
	}

	// Apply time range filter
	if filter.TimeRangeFilter != nil {
		if filter.TimeRangeFilter.StartTime != nil {
			query = query.Where(costsheetv2.CreatedAtGTE(*filter.TimeRangeFilter.StartTime))
		}
		if filter.TimeRangeFilter.EndTime != nil {
			query = query.Where(costsheetv2.CreatedAtLTE(*filter.TimeRangeFilter.EndTime))
		}
	}

	// Apply pagination
	if !filter.IsUnlimited() {
		query = query.Limit(filter.GetLimit()).Offset(filter.GetOffset())
	}

	// Apply sorting
	order := ent.Asc(costsheetv2.FieldCreatedAt)
	if filter.GetSort() != "" {
		switch filter.GetSort() {
		case "name":
			if filter.GetOrder() == "desc" {
				order = ent.Desc(costsheetv2.FieldName)
			} else {
				order = ent.Asc(costsheetv2.FieldName)
			}
		case "created_at":
			if filter.GetOrder() == "desc" {
				order = ent.Desc(costsheetv2.FieldCreatedAt)
			} else {
				order = ent.Asc(costsheetv2.FieldCreatedAt)
			}
		case "updated_at":
			if filter.GetOrder() == "desc" {
				order = ent.Desc(costsheetv2.FieldUpdatedAt)
			} else {
				order = ent.Asc(costsheetv2.FieldUpdatedAt)
			}
		}
	}
	query = query.Order(order)

	entCostsheets, err := query.All(ctx)
	if err != nil {
		r.logger.Errorw("Failed to list costsheet v2 records", "error", err)
		return nil, ierr.WithError(err).
			WithHint("Failed to retrieve costsheet v2 records").
			Mark(ierr.ErrDatabase)
	}

	costsheets := make([]*domainCostsheetV2.CostsheetV2, len(entCostsheets))
	for i, entCostsheet := range entCostsheets {
		costsheets[i] = r.entToDomain(entCostsheet)
	}

	return costsheets, nil
}

// Count returns the total number of costsheet v2 records matching the filter.
func (r *costsheetV2Repository) Count(ctx context.Context, filter *types.CostsheetV2Filter) (int64, error) {
	query := r.client.Reader(ctx).CostsheetV2.Query()

	// Apply tenant and environment from context (like plan repository)
	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)
	if tenantID != "" {
		query = query.Where(costsheetv2.TenantID(tenantID))
	}
	if environmentID != "" {
		query = query.Where(costsheetv2.EnvironmentID(environmentID))
	}

	// Apply status filter if provided
	if filter.GetStatus() != "" {
		query = query.Where(costsheetv2.Status(filter.GetStatus()))
	}

	// Apply costsheet v2 IDs filter
	if len(filter.CostsheetV2IDs) > 0 {
		query = query.Where(costsheetv2.IDIn(filter.CostsheetV2IDs...))
	}

	// Apply time range filter
	if filter.TimeRangeFilter != nil {
		if filter.TimeRangeFilter.StartTime != nil {
			query = query.Where(costsheetv2.CreatedAtGTE(*filter.TimeRangeFilter.StartTime))
		}
		if filter.TimeRangeFilter.EndTime != nil {
			query = query.Where(costsheetv2.CreatedAtLTE(*filter.TimeRangeFilter.EndTime))
		}
	}

	count, err := query.Count(ctx)
	if err != nil {
		r.logger.Errorw("Failed to count costsheet v2 records", "error", err)
		return 0, ierr.WithError(err).
			WithHint("Failed to count costsheet v2 records").
			Mark(ierr.ErrDatabase)
	}

	return int64(count), nil
}

// Update updates an existing costsheet v2 record.
func (r *costsheetV2Repository) Update(ctx context.Context, costsheet *domainCostsheetV2.CostsheetV2) error {
	entCostsheet := r.domainToEnt(costsheet)

	updateQuery := r.client.Writer(ctx).CostsheetV2.UpdateOneID(entCostsheet.ID).
		SetName(entCostsheet.Name).
		SetStatus(entCostsheet.Status).
		SetUpdatedAt(entCostsheet.UpdatedAt).
		SetNillableUpdatedBy(&entCostsheet.UpdatedBy)

	// Update optional fields
	if entCostsheet.LookupKey != "" {
		updateQuery.SetLookupKey(entCostsheet.LookupKey)
	}
	if entCostsheet.Description != "" {
		updateQuery.SetDescription(entCostsheet.Description)
	}
	if entCostsheet.Metadata != nil {
		updateQuery.SetMetadata(entCostsheet.Metadata)
	}

	err := updateQuery.Exec(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return ierr.NewError("costsheet v2 not found").
				WithHint("No costsheet v2 found with the provided ID").
				WithReportableDetails(map[string]any{"id": costsheet.ID}).
				Mark(ierr.ErrNotFound)
		}
		r.logger.Errorw("Failed to update costsheet v2", "error", err, "costsheet_id", costsheet.ID)
		return ierr.WithError(err).
			WithHint("Failed to update costsheet v2").
			Mark(ierr.ErrDatabase)
	}

	r.logger.Infow("Updated costsheet v2", "costsheet_id", costsheet.ID, "name", costsheet.Name)
	return nil
}

// Delete soft deletes a costsheet v2 record by setting its status to deleted.
func (r *costsheetV2Repository) Delete(ctx context.Context, id string) error {
	err := r.client.Writer(ctx).CostsheetV2.UpdateOneID(id).
		SetStatus(string(types.StatusDeleted)).
		SetUpdatedAt(time.Now().UTC()).
		Exec(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return ierr.NewError("costsheet v2 not found").
				WithHint("No costsheet v2 found with the provided ID").
				WithReportableDetails(map[string]any{"id": id}).
				Mark(ierr.ErrNotFound)
		}
		r.logger.Errorw("Failed to delete costsheet v2", "error", err, "costsheet_id", id)
		return ierr.WithError(err).
			WithHint("Failed to delete costsheet v2").
			Mark(ierr.ErrDatabase)
	}

	r.logger.Infow("Deleted costsheet v2", "costsheet_id", id)
	return nil
}

// HardDelete permanently removes a costsheet v2 record from the database.
func (r *costsheetV2Repository) HardDelete(ctx context.Context, id string) error {
	err := r.client.Writer(ctx).CostsheetV2.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ierr.NewError("costsheet v2 not found").
				WithHint("No costsheet v2 found with the provided ID").
				WithReportableDetails(map[string]any{"id": id}).
				Mark(ierr.ErrNotFound)
		}
		r.logger.Errorw("Failed to hard delete costsheet v2", "error", err, "costsheet_id", id)
		return ierr.WithError(err).
			WithHint("Failed to permanently delete costsheet v2").
			Mark(ierr.ErrDatabase)
	}

	r.logger.Infow("Hard deleted costsheet v2", "costsheet_id", id)
	return nil
}

// Exists checks if a costsheet v2 record exists with the given ID.
func (r *costsheetV2Repository) Exists(ctx context.Context, id string) (bool, error) {
	exists, err := r.client.Reader(ctx).CostsheetV2.Query().
		Where(costsheetv2.ID(id)).
		Exist(ctx)

	if err != nil {
		r.logger.Errorw("Failed to check if costsheet v2 exists", "error", err, "costsheet_id", id)
		return false, ierr.WithError(err).
			WithHint("Failed to check costsheet v2 existence").
			Mark(ierr.ErrDatabase)
	}

	return exists, nil
}

// GetByName retrieves a costsheet v2 record by name within a tenant and environment.
func (r *costsheetV2Repository) GetByName(ctx context.Context, tenantID, environmentID, name string) (*domainCostsheetV2.CostsheetV2, error) {
	entCostsheet, err := r.client.Reader(ctx).CostsheetV2.Query().
		Where(
			costsheetv2.TenantID(tenantID),
			costsheetv2.EnvironmentID(environmentID),
			costsheetv2.Name(name),
			costsheetv2.Status(string(types.StatusPublished)),
		).
		First(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ierr.NewError("costsheet v2 not found").
				WithHint("No costsheet v2 found with the provided name").
				WithReportableDetails(map[string]any{
					"name":           name,
					"tenant_id":      tenantID,
					"environment_id": environmentID,
				}).
				Mark(ierr.ErrNotFound)
		}
		r.logger.Errorw("Failed to get costsheet v2 by name",
			"error", err, "name", name, "tenant_id", tenantID, "environment_id", environmentID)
		return nil, ierr.WithError(err).
			WithHint("Failed to retrieve costsheet v2 by name").
			Mark(ierr.ErrDatabase)
	}

	return r.entToDomain(entCostsheet), nil
}

// GetByLookupKey retrieves a costsheet v2 record by lookup key within a tenant and environment.
func (r *costsheetV2Repository) GetByLookupKey(ctx context.Context, tenantID, environmentID, lookupKey string) (*domainCostsheetV2.CostsheetV2, error) {
	entCostsheet, err := r.client.Reader(ctx).CostsheetV2.Query().
		Where(
			costsheetv2.TenantID(tenantID),
			costsheetv2.EnvironmentID(environmentID),
			costsheetv2.LookupKey(lookupKey),
			costsheetv2.Status(string(types.StatusPublished)),
		).
		First(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ierr.NewError("costsheet v2 not found").
				WithHint("No costsheet v2 found with the provided lookup key").
				WithReportableDetails(map[string]any{
					"lookup_key":     lookupKey,
					"tenant_id":      tenantID,
					"environment_id": environmentID,
				}).
				Mark(ierr.ErrNotFound)
		}
		r.logger.Errorw("Failed to get costsheet v2 by lookup key",
			"error", err, "lookup_key", lookupKey, "tenant_id", tenantID, "environment_id", environmentID)
		return nil, ierr.WithError(err).
			WithHint("Failed to retrieve costsheet v2 by lookup key").
			Mark(ierr.ErrDatabase)
	}

	return r.entToDomain(entCostsheet), nil
}

// ListByStatus retrieves costsheet v2 records filtered by status.
func (r *costsheetV2Repository) ListByStatus(ctx context.Context, tenantID, environmentID string, status types.Status) ([]*domainCostsheetV2.CostsheetV2, error) {
	entCostsheets, err := r.client.Reader(ctx).CostsheetV2.Query().
		Where(
			costsheetv2.TenantID(tenantID),
			costsheetv2.EnvironmentID(environmentID),
			costsheetv2.Status(string(status)),
		).
		Order(ent.Asc(costsheetv2.FieldCreatedAt)).
		All(ctx)

	if err != nil {
		r.logger.Errorw("Failed to get costsheet v2 records by status",
			"error", err, "tenant_id", tenantID, "environment_id", environmentID, "status", status)
		return nil, ierr.WithError(err).
			WithHint("Failed to retrieve costsheet v2 records by status").
			Mark(ierr.ErrDatabase)
	}

	costsheets := make([]*domainCostsheetV2.CostsheetV2, len(entCostsheets))
	for i, entCostsheet := range entCostsheets {
		costsheets[i] = r.entToDomain(entCostsheet)
	}

	return costsheets, nil
}

// domainToEnt converts a domain costsheet v2 to an Ent costsheet v2.
func (r *costsheetV2Repository) domainToEnt(costsheet *domainCostsheetV2.CostsheetV2) *ent.CostsheetV2 {
	return &ent.CostsheetV2{
		ID:            costsheet.ID,
		TenantID:      costsheet.TenantID,
		EnvironmentID: costsheet.EnvironmentID,
		Status:        string(costsheet.Status),
		CreatedAt:     costsheet.CreatedAt,
		UpdatedAt:     costsheet.UpdatedAt,
		CreatedBy:     costsheet.CreatedBy,
		UpdatedBy:     costsheet.UpdatedBy,
		Name:          costsheet.Name,
		LookupKey:     costsheet.LookupKey,
		Description:   costsheet.Description,
		Metadata:      costsheet.Metadata,
	}
}

// entToDomain converts an Ent costsheet v2 to a domain costsheet v2.
func (r *costsheetV2Repository) entToDomain(entCostsheet *ent.CostsheetV2) *domainCostsheetV2.CostsheetV2 {
	return &domainCostsheetV2.CostsheetV2{
		ID:            entCostsheet.ID,
		Name:          entCostsheet.Name,
		LookupKey:     entCostsheet.LookupKey,
		Description:   entCostsheet.Description,
		Metadata:      entCostsheet.Metadata,
		EnvironmentID: entCostsheet.EnvironmentID,
		BaseModel: types.BaseModel{
			TenantID:  entCostsheet.TenantID,
			Status:    types.Status(entCostsheet.Status),
			CreatedAt: entCostsheet.CreatedAt,
			UpdatedAt: entCostsheet.UpdatedAt,
			CreatedBy: entCostsheet.CreatedBy,
			UpdatedBy: entCostsheet.UpdatedBy,
		},
	}
}
