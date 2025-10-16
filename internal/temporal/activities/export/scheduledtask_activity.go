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

// IntervalBoundaryCalculator defines the interface for calculating interval boundaries
type IntervalBoundaryCalculator interface {
	CalculateIntervalBoundaries(currentTime time.Time, interval types.ScheduledTaskInterval) (startTime, endTime time.Time)
}

// ScheduledTaskActivity handles scheduled task operations
type ScheduledTaskActivity struct {
	scheduledTaskRepo  scheduledtask.Repository
	taskRepo           task.Repository
	logger             *logger.Logger
	boundaryCalculator IntervalBoundaryCalculator
}

// NewScheduledTaskActivity creates a new scheduled task activity
func NewScheduledTaskActivity(
	scheduledTaskRepo scheduledtask.Repository,
	taskRepo task.Repository,
	logger *logger.Logger,
	boundaryCalculator IntervalBoundaryCalculator,
) *ScheduledTaskActivity {
	return &ScheduledTaskActivity{
		scheduledTaskRepo:  scheduledTaskRepo,
		taskRepo:           taskRepo,
		logger:             logger,
		boundaryCalculator: boundaryCalculator,
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

	// Calculate time range using interval boundaries
	currentTime := time.Now()

	// Calculate start and end times
	// For first run: uses interval boundaries
	// For subsequent runs: uses incremental sync (last export's end_time as new start_time)
	startTime, endTime := a.calculateTimeRange(ctx, task, currentTime)

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

// calculateTimeRange determines the start and end time for the export
// Implements incremental sync: uses last successful export's end_time as new start_time
// For first run or when no previous export exists, uses interval-based boundary alignment
func (a *ScheduledTaskActivity) calculateTimeRange(ctx context.Context, task *scheduledtask.ScheduledTask, currentTime time.Time) (time.Time, time.Time) {
	interval := types.ScheduledTaskInterval(task.Interval)

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
				// Incremental sync: Use last export's end_time as start, and calculate the end of the PREVIOUS completed interval
				// Example: If current time is 9:20, previous interval is 9:10-9:20, so endTime = 9:20
				_, endTime := a.boundaryCalculator.CalculateIntervalBoundaries(currentTime, interval)

				a.logger.Infow("using incremental sync - starting from last export's end_time",
					"scheduled_task_id", task.ID,
					"last_export_end_time", lastEndTime,
					"new_start_time", lastEndTime,
					"new_end_time", endTime,
					"duration", endTime.Sub(lastEndTime))
				return lastEndTime, endTime
			}
		} else {
			a.logger.Warnw("last task metadata missing end_time, falling back to interval-based logic",
				"scheduled_task_id", task.ID)
		}
	}

	// First run OR no previous task found - export the completed previous interval
	// Calculate boundaries to get the END of the previous completed interval
	startTime, endTime := a.boundaryCalculator.CalculateIntervalBoundaries(currentTime, interval)

	a.logger.Infow("no previous export found - using completed previous interval (first run)",
		"scheduled_task_id", task.ID,
		"interval", interval,
		"current_time", currentTime,
		"start_time", startTime,
		"end_time", endTime,
		"duration", endTime.Sub(startTime))

	return startTime, endTime
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
