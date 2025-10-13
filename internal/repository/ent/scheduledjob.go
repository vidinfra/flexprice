package ent

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/ent/scheduledjob"
	domainsj "github.com/flexprice/flexprice/internal/domain/scheduledjob"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/types"
)

// ScheduledJobRepository implements the scheduled job repository using ent
type ScheduledJobRepository struct {
	client postgres.IClient
	logger *logger.Logger
}

// NewScheduledJobRepository creates a new scheduled job repository
func NewScheduledJobRepository(client postgres.IClient, logger *logger.Logger) *ScheduledJobRepository {
	return &ScheduledJobRepository{
		client: client,
		logger: logger,
	}
}

// Create creates a new scheduled job
func (r *ScheduledJobRepository) Create(ctx context.Context, job *domainsj.ScheduledJob) error {
	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)

	client := r.client.Querier(ctx)
	create := client.ScheduledJob.
		Create().
		SetID(job.ID).
		SetTenantID(tenantID).
		SetEnvironmentID(environmentID).
		SetConnectionID(job.ConnectionID).
		SetEntityType(job.EntityType).
		SetInterval(job.Interval).
		SetEnabled(job.Enabled).
		SetStatus(job.Status).
		SetCreatedBy(job.CreatedBy).
		SetUpdatedBy(job.UpdatedBy)

	if job.JobConfig != nil {
		create = create.SetJobConfig(job.JobConfig)
	}
	if job.LastRunAt != nil {
		create = create.SetLastRunAt(*job.LastRunAt)
	}
	if job.NextRunAt != nil {
		create = create.SetNextRunAt(*job.NextRunAt)
	}
	if job.LastRunStatus != "" {
		create = create.SetLastRunStatus(job.LastRunStatus)
	}
	if job.LastRunError != "" {
		create = create.SetLastRunError(job.LastRunError)
	}

	_, err := create.Save(ctx)

	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to create scheduled job").
			Mark(ierr.ErrDatabase)
	}

	return nil
}

// Get retrieves a scheduled job by ID
func (r *ScheduledJobRepository) Get(ctx context.Context, id string) (*domainsj.ScheduledJob, error) {
	client := r.client.Querier(ctx)
	job, err := client.ScheduledJob.
		Query().
		Where(
			scheduledjob.ID(id),
			scheduledjob.TenantID(types.GetTenantID(ctx)),
			scheduledjob.EnvironmentID(types.GetEnvironmentID(ctx)),
		).
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ierr.NewError("scheduled job not found").
				WithHint("Scheduled job with given ID not found").
				Mark(ierr.ErrNotFound)
		}
		return nil, ierr.WithError(err).
			WithHint("Failed to get scheduled job").
			Mark(ierr.ErrDatabase)
	}

	return r.entToModel(job), nil
}

// Update updates an existing scheduled job
func (r *ScheduledJobRepository) Update(ctx context.Context, job *domainsj.ScheduledJob) error {
	client := r.client.Querier(ctx)
	update := client.ScheduledJob.
		UpdateOneID(job.ID).
		SetConnectionID(job.ConnectionID).
		SetEntityType(job.EntityType).
		SetInterval(job.Interval).
		SetEnabled(job.Enabled).
		SetStatus(job.Status).
		SetUpdatedBy(job.UpdatedBy)

	if job.JobConfig != nil {
		update = update.SetJobConfig(job.JobConfig)
	}
	if job.LastRunAt != nil {
		update = update.SetLastRunAt(*job.LastRunAt)
	}
	if job.NextRunAt != nil {
		update = update.SetNextRunAt(*job.NextRunAt)
	}
	if job.LastRunStatus != "" {
		update = update.SetLastRunStatus(job.LastRunStatus)
	}
	if job.LastRunError != "" {
		update = update.SetLastRunError(job.LastRunError)
	}

	err := update.Exec(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return ierr.NewError("scheduled job not found").
				WithHint("Scheduled job with given ID not found").
				Mark(ierr.ErrNotFound)
		}
		return ierr.WithError(err).
			WithHint("Failed to update scheduled job").
			Mark(ierr.ErrDatabase)
	}

	return nil
}

// Delete deletes a scheduled job
func (r *ScheduledJobRepository) Delete(ctx context.Context, id string) error {
	client := r.client.Querier(ctx)
	err := client.ScheduledJob.
		DeleteOneID(id).
		Exec(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return ierr.NewError("scheduled job not found").
				WithHint("Scheduled job with given ID not found").
				Mark(ierr.ErrNotFound)
		}
		return ierr.WithError(err).
			WithHint("Failed to delete scheduled job").
			Mark(ierr.ErrDatabase)
	}

	return nil
}

// List retrieves all scheduled jobs with optional filters
func (r *ScheduledJobRepository) List(ctx context.Context, filters *domainsj.ListFilters) ([]*domainsj.ScheduledJob, error) {
	client := r.client.Querier(ctx)
	query := client.ScheduledJob.Query()

	// Apply tenant and environment filters
	tenantID := filters.TenantID
	if tenantID == "" {
		tenantID = types.GetTenantID(ctx)
	}
	environmentID := filters.EnvironmentID
	if environmentID == "" {
		environmentID = types.GetEnvironmentID(ctx)
	}

	query = query.Where(
		scheduledjob.TenantID(tenantID),
		scheduledjob.EnvironmentID(environmentID),
	)

	// Apply optional filters
	if filters.ConnectionID != "" {
		query = query.Where(scheduledjob.ConnectionID(filters.ConnectionID))
	}
	if filters.EntityType != "" {
		query = query.Where(scheduledjob.EntityType(filters.EntityType))
	}
	if filters.Interval != "" {
		query = query.Where(scheduledjob.Interval(filters.Interval))
	}
	if filters.Enabled != nil {
		query = query.Where(scheduledjob.Enabled(*filters.Enabled))
	}

	// Apply pagination
	if filters.Limit > 0 {
		query = query.Limit(filters.Limit)
	}
	if filters.Offset > 0 {
		query = query.Offset(filters.Offset)
	}

	jobs, err := query.All(ctx)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to list scheduled jobs").
			Mark(ierr.ErrDatabase)
	}

	return r.entSliceToModel(jobs), nil
}

// GetByConnection retrieves all scheduled jobs for a specific connection
func (r *ScheduledJobRepository) GetByConnection(ctx context.Context, connectionID string) ([]*domainsj.ScheduledJob, error) {
	client := r.client.Querier(ctx)
	jobs, err := client.ScheduledJob.
		Query().
		Where(
			scheduledjob.ConnectionID(connectionID),
			scheduledjob.TenantID(types.GetTenantID(ctx)),
			scheduledjob.EnvironmentID(types.GetEnvironmentID(ctx)),
		).
		All(ctx)

	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to get scheduled jobs by connection").
			Mark(ierr.ErrDatabase)
	}

	return r.entSliceToModel(jobs), nil
}

// GetByEntityType retrieves all enabled scheduled jobs for a specific entity type
func (r *ScheduledJobRepository) GetByEntityType(ctx context.Context, entityType string) ([]*domainsj.ScheduledJob, error) {
	client := r.client.Querier(ctx)
	jobs, err := client.ScheduledJob.
		Query().
		Where(
			scheduledjob.EntityType(entityType),
			scheduledjob.Enabled(true),
			scheduledjob.StatusEQ("published"),
			scheduledjob.TenantID(types.GetTenantID(ctx)),
			scheduledjob.EnvironmentID(types.GetEnvironmentID(ctx)),
		).
		All(ctx)

	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to get scheduled jobs by entity type").
			Mark(ierr.ErrDatabase)
	}

	return r.entSliceToModel(jobs), nil
}

// GetJobsDueForExecution retrieves all enabled jobs that are due for execution
func (r *ScheduledJobRepository) GetJobsDueForExecution(ctx context.Context, currentTime time.Time) ([]*domainsj.ScheduledJob, error) {
	client := r.client.Querier(ctx)
	jobs, err := client.ScheduledJob.
		Query().
		Where(
			scheduledjob.Enabled(true),
			scheduledjob.StatusEQ("published"),
			scheduledjob.Or(
				scheduledjob.NextRunAtIsNil(),
				scheduledjob.NextRunAtLTE(currentTime),
			),
			scheduledjob.TenantID(types.GetTenantID(ctx)),
			scheduledjob.EnvironmentID(types.GetEnvironmentID(ctx)),
		).
		All(ctx)

	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to get jobs due for execution").
			Mark(ierr.ErrDatabase)
	}

	return r.entSliceToModel(jobs), nil
}

// UpdateLastRun updates the last run information for a scheduled job
func (r *ScheduledJobRepository) UpdateLastRun(ctx context.Context, jobID string, runTime time.Time, nextRunTime time.Time, status string, errorMsg string) error {
	client := r.client.Querier(ctx)
	update := client.ScheduledJob.
		UpdateOneID(jobID).
		SetLastRunAt(runTime).
		SetNextRunAt(nextRunTime).
		SetLastRunStatus(status)

	if errorMsg != "" {
		update = update.SetLastRunError(errorMsg)
	} else {
		update = update.ClearLastRunError()
	}

	err := update.Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ierr.NewError("scheduled job not found").
				WithHint("Scheduled job with given ID not found").
				Mark(ierr.ErrNotFound)
		}
		return ierr.WithError(err).
			WithHint("Failed to update last run").
			Mark(ierr.ErrDatabase)
	}

	return nil
}

// entToModel converts an ent ScheduledJob to domain model
func (r *ScheduledJobRepository) entToModel(entJob *ent.ScheduledJob) *domainsj.ScheduledJob {
	if entJob == nil {
		return nil
	}

	return &domainsj.ScheduledJob{
		ID:            entJob.ID,
		TenantID:      entJob.TenantID,
		EnvironmentID: entJob.EnvironmentID,
		ConnectionID:  entJob.ConnectionID,
		EntityType:    entJob.EntityType,
		Interval:      entJob.Interval,
		Enabled:       entJob.Enabled,
		JobConfig:     entJob.JobConfig,
		LastRunAt:     entJob.LastRunAt,
		NextRunAt:     entJob.NextRunAt,
		LastRunStatus: entJob.LastRunStatus,
		LastRunError:  entJob.LastRunError,
		Status:        entJob.Status,
		CreatedAt:     entJob.CreatedAt,
		UpdatedAt:     entJob.UpdatedAt,
		CreatedBy:     entJob.CreatedBy,
		UpdatedBy:     entJob.UpdatedBy,
	}
}

// entSliceToModel converts a slice of ent ScheduledJobs to domain models
func (r *ScheduledJobRepository) entSliceToModel(entJobs []*ent.ScheduledJob) []*domainsj.ScheduledJob {
	if entJobs == nil {
		return nil
	}

	jobs := make([]*domainsj.ScheduledJob, len(entJobs))
	for i, entJob := range entJobs {
		jobs[i] = r.entToModel(entJob)
	}
	return jobs
}
