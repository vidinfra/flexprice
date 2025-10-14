package export

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/domain/scheduledjob"
	"github.com/flexprice/flexprice/internal/domain/task"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
)

// ScheduledJobActivity handles scheduled job operations
type ScheduledJobActivity struct {
	scheduledJobRepo scheduledjob.Repository
	taskRepo         task.Repository
	logger           *logger.Logger
}

// NewScheduledJobActivity creates a new scheduled job activity
func NewScheduledJobActivity(
	scheduledJobRepo scheduledjob.Repository,
	taskRepo task.Repository,
	logger *logger.Logger,
) *ScheduledJobActivity {
	return &ScheduledJobActivity{
		scheduledJobRepo: scheduledJobRepo,
		taskRepo:         taskRepo,
		logger:           logger,
	}
}

// GetScheduledJobDetailsInput represents input for getting job details
type GetScheduledJobDetailsInput struct {
	ScheduledJobID string
	TenantID       string
	EnvID          string
}

// ScheduledJobDetails contains job details needed for export
type ScheduledJobDetails struct {
	ScheduledJobID string
	TenantID       string
	EnvID          string
	EntityType     string
	Enabled        bool
	ConnectionID   string
	StartTime      time.Time
	EndTime        time.Time
	JobConfig      *types.S3JobConfig
}

// GetScheduledJobDetails fetches scheduled job and calculates time range
func (a *ScheduledJobActivity) GetScheduledJobDetails(ctx context.Context, input GetScheduledJobDetailsInput) (*ScheduledJobDetails, error) {
	a.logger.Infow("fetching scheduled job details",
		"scheduled_job_id", input.ScheduledJobID,
		"tenant_id", input.TenantID,
		"env_id", input.EnvID)

	// Add tenant and env to context for repository query
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvID)

	// Get scheduled job
	job, err := a.scheduledJobRepo.Get(ctx, input.ScheduledJobID)
	if err != nil {
		a.logger.Errorw("failed to get scheduled job", "error", err)
		return nil, ierr.WithError(err).
			WithHint("Failed to fetch scheduled job").
			Mark(ierr.ErrDatabase)
	}

	// Parse job config
	jobConfig, err := job.GetS3JobConfig()
	if err != nil {
		a.logger.Errorw("failed to parse job config", "error", err)
		return nil, ierr.WithError(err).
			WithHint("Invalid job configuration").
			Mark(ierr.ErrValidation)
	}

	// Calculate time range
	endTime := time.Now()
	startTime := a.calculateStartTime(ctx, job, endTime)

	a.logger.Infow("scheduled job details retrieved",
		"scheduled_job_id", input.ScheduledJobID,
		"entity_type", job.EntityType,
		"interval", job.Interval,
		"start_time", startTime,
		"end_time", endTime)

	return &ScheduledJobDetails{
		ScheduledJobID: job.ID,
		TenantID:       job.TenantID,
		EnvID:          job.EnvironmentID,
		EntityType:     job.EntityType,
		Enabled:        job.Enabled,
		ConnectionID:   job.ConnectionID,
		StartTime:      startTime,
		EndTime:        endTime,
		JobConfig:      jobConfig,
	}, nil
}

// calculateStartTime determines the start time for the export
// Implements incremental sync: uses last successful export's end_time as new start_time
func (a *ScheduledJobActivity) calculateStartTime(ctx context.Context, job *scheduledjob.ScheduledJob, endTime time.Time) time.Time {
	a.logger.Infow("calculating start time for export",
		"scheduled_job_id", job.ID,
		"interval", job.Interval,
		"end_time", endTime)

	// Try to get last successful export task for incremental sync
	lastTask, err := a.taskRepo.GetLastSuccessfulExportTask(ctx, job.ID)
	if err != nil {
		// Log error but continue with interval-based logic
		a.logger.Warnw("error getting last successful export, falling back to interval-based logic",
			"scheduled_job_id", job.ID,
			"error", err)
	}

	// If we found a previous successful export, use its end_time as our start_time (incremental sync)
	if lastTask != nil && lastTask.Metadata != nil {
		if endTimeStr, ok := lastTask.Metadata["end_time"].(string); ok {
			lastEndTime, err := time.Parse(time.RFC3339, endTimeStr)
			if err != nil {
				a.logger.Warnw("failed to parse last task end_time, falling back to interval-based logic",
					"scheduled_job_id", job.ID,
					"end_time_str", endTimeStr,
					"error", err)
			} else {
				a.logger.Infow("using incremental sync - starting from last export's end_time",
					"scheduled_job_id", job.ID,
					"last_export_end_time", lastEndTime,
					"new_start_time", lastEndTime,
					"new_end_time", endTime,
					"duration", endTime.Sub(lastEndTime))
				return lastEndTime
			}
		} else {
			a.logger.Warnw("last task metadata missing end_time, falling back to interval-based logic",
				"scheduled_job_id", job.ID)
		}
	}

	// First run OR no previous task found - use interval-based logic
	interval := types.ScheduledJobInterval(job.Interval)
	var startTime time.Time

	switch interval {
	case types.ScheduledJobIntervalHourly:
		startTime = endTime.Add(-1 * time.Hour)
	case types.ScheduledJobIntervalDaily:
		startTime = endTime.Add(-24 * time.Hour)
	case types.ScheduledJobIntervalWeekly:
		startTime = endTime.Add(-7 * 24 * time.Hour)
	case types.ScheduledJobIntervalMonthly:
		startTime = endTime.AddDate(0, -1, 0)
	default:
		// Default to daily
		startTime = endTime.Add(-24 * time.Hour)
	}

	a.logger.Infow("no previous export found - using interval-based logic (first run)",
		"scheduled_job_id", job.ID,
		"interval", interval,
		"start_time", startTime,
		"end_time", endTime,
		"duration", endTime.Sub(startTime))

	return startTime
}

// UpdateScheduledJobInput represents input for updating scheduled job
type UpdateScheduledJobInput struct {
	ScheduledJobID string
	TenantID       string
	EnvID          string
	LastRunAt      time.Time
	LastRunStatus  string
}

// UpdateScheduledJobLastRun updates the last run timestamp of a scheduled job
func (a *ScheduledJobActivity) UpdateScheduledJobLastRun(ctx context.Context, input UpdateScheduledJobInput) error {
	a.logger.Infow("updating scheduled job last run",
		"scheduled_job_id", input.ScheduledJobID,
		"tenant_id", input.TenantID,
		"env_id", input.EnvID,
		"last_run_at", input.LastRunAt,
		"status", input.LastRunStatus)

	// Add tenant and env to context for repository query
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvID)

	job, err := a.scheduledJobRepo.Get(ctx, input.ScheduledJobID)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to get scheduled job").
			Mark(ierr.ErrDatabase)
	}

	// Update last run fields
	job.LastRunAt = &input.LastRunAt
	job.LastRunStatus = input.LastRunStatus

	// Calculate next run time
	nextRun := job.CalculateNextRunTime(input.LastRunAt)
	job.NextRunAt = &nextRun

	err = a.scheduledJobRepo.Update(ctx, job)
	if err != nil {
		a.logger.Errorw("failed to update scheduled job", "error", err)
		return ierr.WithError(err).
			WithHint("Failed to update scheduled job").
			Mark(ierr.ErrDatabase)
	}

	a.logger.Infow("scheduled job updated",
		"scheduled_job_id", input.ScheduledJobID,
		"next_run_at", nextRun)

	return nil
}
