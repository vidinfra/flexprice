package export

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/domain/scheduledtask"
	"github.com/flexprice/flexprice/internal/domain/task"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
)

// ScheduledTaskActivity handles scheduled task operations
type ScheduledTaskActivity struct {
	scheduledTaskRepo scheduledtask.Repository
	taskRepo          task.Repository
	logger            *logger.Logger
}

// NewScheduledTaskActivity creates a new scheduled task activity
func NewScheduledTaskActivity(
	scheduledTaskRepo scheduledtask.Repository,
	taskRepo task.Repository,
	logger *logger.Logger,
) *ScheduledTaskActivity {
	return &ScheduledTaskActivity{
		scheduledTaskRepo: scheduledTaskRepo,
		taskRepo:          taskRepo,
		logger:            logger,
	}
}

// GetScheduledTaskDetailsInput represents input for getting task details
type GetScheduledTaskDetailsInput struct {
	ScheduledTaskID string
	TenantID        string
	EnvID           string
}

// ScheduledTaskDetails contains task details needed for export
type ScheduledTaskDetails struct {
	ScheduledTaskID string
	TenantID        string
	EnvID           string
	EntityType      string
	Enabled         bool
	ConnectionID    string
	StartTime       time.Time
	EndTime         time.Time
	JobConfig       *types.S3JobConfig
}

// GetScheduledTaskDetails fetches scheduled task and calculates time range
func (a *ScheduledTaskActivity) GetScheduledTaskDetails(ctx context.Context, input GetScheduledTaskDetailsInput) (*ScheduledTaskDetails, error) {
	a.logger.Infow("fetching scheduled task details",
		"scheduled_task_id", input.ScheduledTaskID,
		"tenant_id", input.TenantID,
		"env_id", input.EnvID)

	// Add tenant and env to context for repository query
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvID)

	// Get scheduled task
	task, err := a.scheduledTaskRepo.Get(ctx, input.ScheduledTaskID)
	if err != nil {
		a.logger.Errorw("failed to get scheduled task", "error", err)
		return nil, ierr.WithError(err).
			WithHint("Failed to fetch scheduled task").
			Mark(ierr.ErrDatabase)
	}

	// Parse job config
	jobConfig, err := task.GetS3JobConfig()
	if err != nil {
		a.logger.Errorw("failed to parse job config", "error", err)
		return nil, ierr.WithError(err).
			WithHint("Invalid job configuration").
			Mark(ierr.ErrValidation)
	}

	// Calculate time range
	endTime := time.Now()
	startTime := a.calculateStartTime(ctx, task, endTime)

	a.logger.Infow("scheduled task details retrieved",
		"scheduled_task_id", input.ScheduledTaskID,
		"entity_type", task.EntityType,
		"interval", task.Interval,
		"start_time", startTime,
		"end_time", endTime)

	return &ScheduledTaskDetails{
		ScheduledTaskID: task.ID,
		TenantID:        task.TenantID,
		EnvID:           task.EnvironmentID,
		EntityType:      task.EntityType,
		Enabled:         task.Enabled,
		ConnectionID:    task.ConnectionID,
		StartTime:       startTime,
		EndTime:         endTime,
		JobConfig:       jobConfig,
	}, nil
}

// calculateStartTime determines the start time for the export
// Implements incremental sync: uses last successful export's end_time as new start_time
func (a *ScheduledTaskActivity) calculateStartTime(ctx context.Context, task *scheduledtask.ScheduledTask, endTime time.Time) time.Time {
	a.logger.Infow("calculating start time for export",
		"scheduled_task_id", task.ID,
		"interval", task.Interval,
		"end_time", endTime)

	// Try to get last successful export task for incremental sync
	lastTask, err := a.taskRepo.GetLastSuccessfulExportTask(ctx, task.ID)
	if err != nil {
		// Log error but continue with interval-based logic
		a.logger.Warnw("error getting last successful export, falling back to interval-based logic",
			"scheduled_task_id", task.ID,
			"error", err)
	}

	// If we found a previous successful export, use its end_time as our start_time (incremental sync)
	if lastTask != nil && lastTask.Metadata != nil {
		if endTimeStr, ok := lastTask.Metadata["end_time"].(string); ok {
			lastEndTime, err := time.Parse(time.RFC3339, endTimeStr)
			if err != nil {
				a.logger.Warnw("failed to parse last task end_time, falling back to interval-based logic",
					"scheduled_task_id", task.ID,
					"end_time_str", endTimeStr,
					"error", err)
			} else {
				a.logger.Infow("using incremental sync - starting from last export's end_time",
					"scheduled_task_id", task.ID,
					"last_export_end_time", lastEndTime,
					"new_start_time", lastEndTime,
					"new_end_time", endTime,
					"duration", endTime.Sub(lastEndTime))
				return lastEndTime
			}
		} else {
			a.logger.Warnw("last task metadata missing end_time, falling back to interval-based logic",
				"scheduled_task_id", task.ID)
		}
	}

	// First run OR no previous task found - use interval-based logic
	interval := types.ScheduledTaskInterval(task.Interval)
	var startTime time.Time

	switch interval {
	case types.ScheduledTaskIntervalTesting:
		startTime = endTime.Add(-10 * time.Minute)
	case types.ScheduledTaskIntervalHourly:
		startTime = endTime.Add(-1 * time.Hour)
	case types.ScheduledTaskIntervalDaily:
		startTime = endTime.Add(-24 * time.Hour)
	case types.ScheduledTaskIntervalWeekly:
		startTime = endTime.Add(-7 * 24 * time.Hour)
	case types.ScheduledTaskIntervalMonthly:
		startTime = endTime.AddDate(0, -1, 0)
	default:
		// Default to daily
		startTime = endTime.Add(-24 * time.Hour)
	}

	a.logger.Infow("no previous export found - using interval-based logic (first run)",
		"scheduled_task_id", task.ID,
		"interval", interval,
		"start_time", startTime,
		"end_time", endTime,
		"duration", endTime.Sub(startTime))

	return startTime
}

// UpdateScheduledTaskInput represents input for updating scheduled task
type UpdateScheduledTaskInput struct {
	ScheduledTaskID string
	TenantID        string
	EnvID           string
	LastRunAt       time.Time
	LastRunStatus   string
	LastRunError    string
}

// UpdateScheduledTaskLastRun updates the last run timestamp of a scheduled task
func (a *ScheduledTaskActivity) UpdateScheduledTaskLastRun(ctx context.Context, input UpdateScheduledTaskInput) error {
	a.logger.Infow("updating scheduled task last run",
		"scheduled_task_id", input.ScheduledTaskID,
		"tenant_id", input.TenantID,
		"env_id", input.EnvID,
		"last_run_at", input.LastRunAt,
		"status", input.LastRunStatus)

	// Add tenant and env to context for repository query
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvID)

	task, err := a.scheduledTaskRepo.Get(ctx, input.ScheduledTaskID)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to get scheduled task").
			Mark(ierr.ErrDatabase)
	}

	// Update last run fields
	task.LastRunAt = &input.LastRunAt
	task.LastRunStatus = input.LastRunStatus
	task.LastRunError = input.LastRunError
	task.UpdatedAt = time.Now()
	task.UpdatedBy = types.GetUserID(ctx)

	// Calculate next run time
	nextRun := task.CalculateNextRunTime(input.LastRunAt)
	task.NextRunAt = &nextRun

	err = a.scheduledTaskRepo.Update(ctx, task)
	if err != nil {
		a.logger.Errorw("failed to update scheduled task", "error", err)
		return ierr.WithError(err).
			WithHint("Failed to update scheduled task").
			Mark(ierr.ErrDatabase)
	}

	a.logger.Infow("scheduled task updated",
		"scheduled_task_id", input.ScheduledTaskID,
		"next_run_at", nextRun)

	return nil
}
