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

// ScheduledJobDetails contains job details needed for export
type ScheduledJobDetails struct {
	ScheduledJobID string
	TenantID       string
	EnvID          string
	EntityType     string
	Enabled        bool
	StartTime      time.Time
	EndTime        time.Time
	JobConfig      *types.S3JobConfig
}

// GetScheduledJobDetails fetches scheduled job and calculates time range
func (a *ScheduledJobActivity) GetScheduledJobDetails(ctx context.Context, scheduledJobID string) (*ScheduledJobDetails, error) {
	a.logger.Infow("fetching scheduled job details", "scheduled_job_id", scheduledJobID)

	// Get scheduled job
	job, err := a.scheduledJobRepo.Get(ctx, scheduledJobID)
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
		"scheduled_job_id", scheduledJobID,
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
		StartTime:      startTime,
		EndTime:        endTime,
		JobConfig:      jobConfig,
	}, nil
}

// calculateStartTime determines the start time for the export
func (a *ScheduledJobActivity) calculateStartTime(ctx context.Context, job *scheduledjob.ScheduledJob, endTime time.Time) time.Time {
	// TODO: Query task table to get last completed task's end_time
	// For now, use simple logic based on interval

	// If this is the first run OR we can't find last task, use interval-based logic
	interval := types.ScheduledJobInterval(job.Interval)

	switch interval {
	case types.ScheduledJobIntervalHourly:
		return endTime.Add(-1 * time.Hour)
	case types.ScheduledJobIntervalDaily:
		return endTime.Add(-24 * time.Hour)
	case types.ScheduledJobIntervalWeekly:
		return endTime.Add(-7 * 24 * time.Hour)
	case types.ScheduledJobIntervalMonthly:
		return endTime.AddDate(0, -1, 0)
	default:
		// Default to daily
		return endTime.Add(-24 * time.Hour)
	}
}

// UpdateScheduledJobInput represents input for updating scheduled job
type UpdateScheduledJobInput struct {
	ScheduledJobID string
	LastRunAt      time.Time
	LastRunStatus  string
}

// UpdateScheduledJobLastRun updates the last run timestamp of a scheduled job
func (a *ScheduledJobActivity) UpdateScheduledJobLastRun(ctx context.Context, input UpdateScheduledJobInput) error {
	a.logger.Infow("updating scheduled job last run",
		"scheduled_job_id", input.ScheduledJobID,
		"last_run_at", input.LastRunAt,
		"status", input.LastRunStatus)

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
