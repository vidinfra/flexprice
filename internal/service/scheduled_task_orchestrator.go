package service

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/domain/scheduledtask"
	"github.com/flexprice/flexprice/internal/domain/task"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	exportWorkflows "github.com/flexprice/flexprice/internal/temporal/workflows/export"
	"github.com/flexprice/flexprice/internal/types"
	"go.temporal.io/sdk/client"
)

// ScheduledTaskOrchestrator manages Temporal schedules for scheduled tasks
type ScheduledTaskOrchestrator struct {
	scheduledTaskRepo scheduledtask.Repository
	taskRepo          task.Repository
	temporalClient    client.Client
	logger            *logger.Logger
}

// NewScheduledTaskOrchestrator creates a new orchestrator
func NewScheduledTaskOrchestrator(
	scheduledTaskRepo scheduledtask.Repository,
	taskRepo task.Repository,
	temporalClient client.Client,
	logger *logger.Logger,
) *ScheduledTaskOrchestrator {
	return &ScheduledTaskOrchestrator{
		scheduledTaskRepo: scheduledTaskRepo,
		taskRepo:          taskRepo,
		temporalClient:    temporalClient,
		logger:            logger,
	}
}

// StartScheduledTask creates and starts a Temporal schedule for the task
func (o *ScheduledTaskOrchestrator) StartScheduledTask(ctx context.Context, taskID string) error {
	o.logger.Infow("starting scheduled task", "task_id", taskID)

	// Get the task
	task, err := o.scheduledTaskRepo.Get(ctx, taskID)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to get scheduled task").
			Mark(ierr.ErrDatabase)
	}

	// If already has a schedule, unpause it
	if task.TemporalScheduleID != "" {
		o.logger.Infow("temporal schedule already exists, unpausing",
			"schedule_id", task.TemporalScheduleID)

		handle := o.temporalClient.ScheduleClient().GetHandle(ctx, task.TemporalScheduleID)
		err = handle.Unpause(ctx, client.ScheduleUnpauseOptions{})
		if err != nil {
			o.logger.Errorw("failed to unpause schedule", "error", err)
			return ierr.WithError(err).
				WithHint("Failed to unpause Temporal schedule").
				Mark(ierr.ErrInternal)
		}
		return nil
	}

	// Create new Temporal schedule
	// Use the scheduled task ID directly (already has schtask_ prefix)
	scheduleID := task.ID
	cronExpr := o.getCronExpression(types.ScheduledTaskInterval(task.Interval))

	scheduleSpec := client.ScheduleSpec{
		CronExpressions: []string{cronExpr},
	}

	action := &client.ScheduleWorkflowAction{
		// Don't set ID here - let Temporal auto-generate for the wrapper workflow
		// The child workflow (ExecuteExportWorkflow) will use task_id-export format
		Workflow: exportWorkflows.ScheduledExportWorkflow,
		Args: []interface{}{
			exportWorkflows.ScheduledExportWorkflowInput{
				ScheduledTaskID: task.ID,
				TenantID:        task.TenantID,
				EnvID:           task.EnvironmentID,
			},
		},
		TaskQueue: string(types.TemporalTaskQueueExport),
	}

	schedule, err := o.temporalClient.ScheduleClient().Create(ctx, client.ScheduleOptions{
		ID:     scheduleID,
		Spec:   scheduleSpec,
		Action: action,
		Paused: false, // Start immediately
	})

	if err != nil {
		o.logger.Errorw("failed to create temporal schedule", "error", err)
		return ierr.WithError(err).
			WithHint("Failed to create Temporal schedule").
			Mark(ierr.ErrInternal)
	}

	o.logger.Infow("temporal schedule created",
		"schedule_id", scheduleID,
		"cron", cronExpr)

	// Update task with schedule ID
	task.TemporalScheduleID = scheduleID
	err = o.scheduledTaskRepo.Update(ctx, task)
	if err != nil {
		o.logger.Errorw("failed to update task with schedule ID", "error", err)
		// Try to delete the created schedule
		_ = schedule.Delete(ctx)
		return ierr.WithError(err).
			WithHint("Failed to save schedule ID").
			Mark(ierr.ErrDatabase)
	}

	o.logger.Infow("scheduled task started successfully", "task_id", taskID)
	return nil
}

// StopScheduledTask pauses the Temporal schedule
func (o *ScheduledTaskOrchestrator) StopScheduledTask(ctx context.Context, taskID string) error {
	o.logger.Infow("stopping scheduled task", "task_id", taskID)

	task, err := o.scheduledTaskRepo.Get(ctx, taskID)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to get scheduled task").
			Mark(ierr.ErrDatabase)
	}

	if task.TemporalScheduleID == "" {
		o.logger.Infow("no temporal schedule to stop", "task_id", taskID)
		return nil
	}

	// Pause the schedule
	handle := o.temporalClient.ScheduleClient().GetHandle(ctx, task.TemporalScheduleID)
	err = handle.Pause(ctx, client.SchedulePauseOptions{})
	if err != nil {
		o.logger.Errorw("failed to pause schedule", "error", err)
		return ierr.WithError(err).
			WithHint("Failed to pause Temporal schedule").
			Mark(ierr.ErrInternal)
	}

	o.logger.Infow("scheduled task stopped successfully", "task_id", taskID)
	return nil
}

// DeleteScheduledTask deletes the Temporal schedule permanently
func (o *ScheduledTaskOrchestrator) DeleteScheduledTask(ctx context.Context, taskID string) error {
	o.logger.Infow("deleting temporal schedule for scheduled task", "task_id", taskID)

	task, err := o.scheduledTaskRepo.Get(ctx, taskID)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to get scheduled task").
			Mark(ierr.ErrDatabase)
	}

	if task.TemporalScheduleID == "" {
		o.logger.Infow("no temporal schedule to delete", "task_id", taskID)
		return nil
	}

	// Delete the schedule from Temporal
	handle := o.temporalClient.ScheduleClient().GetHandle(ctx, task.TemporalScheduleID)
	err = handle.Delete(ctx)
	if err != nil {
		o.logger.Errorw("failed to delete temporal schedule", "schedule_id", task.TemporalScheduleID, "error", err)
		return ierr.WithError(err).
			WithHint("Failed to delete Temporal schedule").
			Mark(ierr.ErrInternal)
	}

	o.logger.Infow("temporal schedule deleted successfully",
		"task_id", taskID,
		"schedule_id", task.TemporalScheduleID)
	return nil
}

// TriggerForceRun executes the export workflow immediately (bypasses schedule)
// If customStart and customEnd are provided, uses those times. Otherwise, calculates automatically.
// Returns: workflowID, startTime, endTime, mode, error
func (o *ScheduledTaskOrchestrator) TriggerForceRun(ctx context.Context, taskID string, customStart, customEnd *time.Time) (string, time.Time, time.Time, string, error) {
	o.logger.Infow("triggering force run", "task_id", taskID, "custom_start", customStart, "custom_end", customEnd)

	// Get the task
	task, err := o.scheduledTaskRepo.Get(ctx, taskID)
	if err != nil {
		return "", time.Time{}, time.Time{}, "", ierr.WithError(err).
			WithHint("Failed to get scheduled task").
			Mark(ierr.ErrDatabase)
	}

	// Get job config
	jobConfig, err := task.GetS3JobConfig()
	if err != nil {
		return "", time.Time{}, time.Time{}, "", ierr.WithError(err).
			WithHint("Invalid job configuration").
			Mark(ierr.ErrValidation)
	}

	// Determine time range and mode
	var startTime, endTime time.Time
	var mode string

	if customStart != nil && customEnd != nil {
		// User provided custom time range
		startTime = *customStart
		endTime = *customEnd
		mode = "custom"
		o.logger.Infow("using custom time range",
			"start_time", startTime,
			"end_time", endTime,
			"duration", endTime.Sub(startTime))
	} else {
		// Calculate automatically based on interval boundaries
		interval := types.ScheduledTaskInterval(task.Interval)
		currentTime := time.Now()

		// Calculate interval boundaries for force run
		startTime, endTime = o.CalculateIntervalBoundaries(currentTime, interval)
		mode = "automatic"

		o.logger.Infow("using automatic time range based on interval boundaries",
			"start_time", startTime,
			"end_time", endTime,
			"current_time", currentTime,
			"interval", interval,
			"duration", endTime.Sub(startTime))
	}

	// Generate task ID and use it as workflow ID
	// Format: {task_id}-export
	exportTaskID := types.GenerateUUIDWithPrefix("task")
	workflowID := fmt.Sprintf("%s-export", exportTaskID)

	o.logger.Infow("triggering force run export",
		"export_task_id", exportTaskID,
		"workflow_id", workflowID,
		"scheduled_task_id", taskID,
		"mode", mode)

	workflowOptions := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: string(types.TemporalTaskQueueExport),
	}

	input := exportWorkflows.ExecuteExportWorkflowInput{
		TaskID:          exportTaskID,
		ScheduledTaskID: task.ID,
		EntityType:      types.ExportEntityType(task.EntityType),
		ConnectionID:    task.ConnectionID,
		TenantID:        task.TenantID,
		EnvID:           task.EnvironmentID,
		StartTime:       startTime,
		EndTime:         endTime,
		JobConfig:       jobConfig,
	}

	workflowRun, err := o.temporalClient.ExecuteWorkflow(ctx, workflowOptions, exportWorkflows.ExecuteExportWorkflow, input)
	if err != nil {
		o.logger.Errorw("failed to start export workflow", "error", err)
		return "", time.Time{}, time.Time{}, "", ierr.WithError(err).
			WithHint("Failed to start export workflow").
			Mark(ierr.ErrInternal)
	}

	o.logger.Infow("force run triggered",
		"scheduled_task_id", taskID,
		"workflow_id", workflowRun.GetID(),
		"run_id", workflowRun.GetRunID(),
		"start_time", startTime,
		"end_time", endTime,
		"mode", mode)

	return workflowRun.GetID(), startTime, endTime, mode, nil
}

// getCronExpression converts interval to cron expression
func (o *ScheduledTaskOrchestrator) getCronExpression(interval types.ScheduledTaskInterval) string {
	switch interval {
	case types.ScheduledTaskIntervalTesting:
		return "*/10 * * * *" // Every 10 minutes (for testing)
	case types.ScheduledTaskIntervalHourly:
		return "0 * * * *" // Every hour
	case types.ScheduledTaskIntervalDaily:
		return "0 0 * * *" // Every day at midnight
	case types.ScheduledTaskIntervalWeekly:
		return "0 0 * * 1" // Every Monday at midnight (week starts Monday)
	case types.ScheduledTaskIntervalMonthly:
		return "0 0 1 * *" // First day of every month at midnight
	case types.ScheduledTaskIntervalYearly:
		return "0 0 1 1 *" // First day of every year at midnight (Jan 1st)
	default:
		return "0 0 * * *" // Default to daily
	}
}

// CalculateIntervalBoundaries calculates the start and end time based on interval boundaries
// This ensures data is aligned to natural time boundaries (hour, day, week, month, year)
func (o *ScheduledTaskOrchestrator) CalculateIntervalBoundaries(currentTime time.Time, interval types.ScheduledTaskInterval) (startTime, endTime time.Time) {
	switch interval {
	case types.ScheduledTaskIntervalTesting:
		// For testing: align to 10-minute intervals
		// Truncate to the nearest 10 minutes
		minutes := currentTime.Minute() / 10 * 10
		startTime = time.Date(
			currentTime.Year(), currentTime.Month(), currentTime.Day(),
			currentTime.Hour(), minutes, 0, 0, currentTime.Location(),
		)
		endTime = startTime.Add(10 * time.Minute)

	case types.ScheduledTaskIntervalHourly:
		// Align to hour boundaries
		// Example: If current time is 10:30 AM, range = 10:00 AM → 11:00 AM
		startTime = time.Date(
			currentTime.Year(), currentTime.Month(), currentTime.Day(),
			currentTime.Hour(), 0, 0, 0, currentTime.Location(),
		)
		endTime = startTime.Add(1 * time.Hour)

	case types.ScheduledTaskIntervalDaily:
		// Align to day boundaries (midnight to midnight)
		// Example: If run anytime on 16 Oct 2025, range = 16 Oct 00:00 → 17 Oct 00:00
		startTime = time.Date(
			currentTime.Year(), currentTime.Month(), currentTime.Day(),
			0, 0, 0, 0, currentTime.Location(),
		)
		endTime = startTime.AddDate(0, 0, 1) // Add 1 day

	case types.ScheduledTaskIntervalWeekly:
		// Align to week boundaries (Monday 00:00 to next Monday 00:00)
		// Example: If run on Thursday 16 Oct 2025, range = Monday 13 Oct 00:00 → Monday 20 Oct 00:00

		// Get the current weekday (0 = Sunday, 1 = Monday, ..., 6 = Saturday)
		weekday := currentTime.Weekday()

		// Calculate days since last Monday
		var daysSinceMonday int
		if weekday == time.Sunday {
			daysSinceMonday = 6 // Sunday is 6 days after Monday
		} else {
			daysSinceMonday = int(weekday) - 1 // Monday = 0, Tuesday = 1, etc.
		}

		// Get Monday 00:00 of current week
		startTime = time.Date(
			currentTime.Year(), currentTime.Month(), currentTime.Day(),
			0, 0, 0, 0, currentTime.Location(),
		).AddDate(0, 0, -daysSinceMonday)

		// End time is next Monday 00:00 (7 days later)
		endTime = startTime.AddDate(0, 0, 7)

	case types.ScheduledTaskIntervalMonthly:
		// Align to month boundaries (1st of month 00:00 to 1st of next month 00:00)
		// Example: If run on 16 Oct 2025, range = 1 Oct 2025 00:00 → 1 Nov 2025 00:00
		startTime = time.Date(
			currentTime.Year(), currentTime.Month(), 1,
			0, 0, 0, 0, currentTime.Location(),
		)
		endTime = startTime.AddDate(0, 1, 0) // Add 1 month

	case types.ScheduledTaskIntervalYearly:
		// Align to year boundaries (1st Jan 00:00 to 1st Jan next year 00:00)
		// Example: If run anytime in 2025, range = 1 Jan 2025 00:00 → 1 Jan 2026 00:00
		startTime = time.Date(
			currentTime.Year(), time.January, 1,
			0, 0, 0, 0, currentTime.Location(),
		)
		endTime = startTime.AddDate(1, 0, 0) // Add 1 year

	default:
		// Default to daily
		startTime = time.Date(
			currentTime.Year(), currentTime.Month(), currentTime.Day(),
			0, 0, 0, 0, currentTime.Location(),
		)
		endTime = startTime.AddDate(0, 0, 1)
	}

	return startTime, endTime
}
