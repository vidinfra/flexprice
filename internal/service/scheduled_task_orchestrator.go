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

// TriggerManualSync executes the export workflow immediately (bypasses schedule)
// If customStart and customEnd are provided, uses those times. Otherwise, calculates automatically.
// Returns: workflowID, startTime, endTime, mode, error
func (o *ScheduledTaskOrchestrator) TriggerManualSync(ctx context.Context, taskID string, customStart, customEnd *time.Time) (string, time.Time, time.Time, string, error) {
	o.logger.Infow("triggering manual sync", "task_id", taskID, "custom_start", customStart, "custom_end", customEnd)

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
		// Calculate automatically
		endTime = time.Now()
		startTime = o.calculateManualSyncStartTime(ctx, task, endTime)
		mode = "automatic"
		o.logger.Infow("using automatic time range",
			"start_time", startTime,
			"end_time", endTime,
			"duration", endTime.Sub(startTime))
	}

	// Generate task ID and use it as workflow ID
	// Format: {task_id}-export
	exportTaskID := types.GenerateUUIDWithPrefix("task")
	workflowID := fmt.Sprintf("%s-export", exportTaskID)

	o.logger.Infow("triggering manual export",
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

	o.logger.Infow("manual sync triggered",
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
		return "0 0 * * 0" // Every Sunday at midnight
	case types.ScheduledTaskIntervalMonthly:
		return "0 0 1 * *" // First day of every month at midnight
	default:
		return "0 0 * * *" // Default to daily
	}
}

// calculateManualSyncStartTime determines start time for manual sync
// Implements incremental sync: uses last successful export's end_time as new start_time
func (o *ScheduledTaskOrchestrator) calculateManualSyncStartTime(ctx context.Context, task *scheduledtask.ScheduledTask, endTime time.Time) time.Time {
	o.logger.Infow("calculating start time for manual sync",
		"scheduled_task_id", task.ID,
		"interval", task.Interval,
		"end_time", endTime)

	// Try to get last successful export task for incremental sync
	lastTask, err := o.taskRepo.GetLastSuccessfulExportTask(ctx, task.ID)
	if err != nil {
		// Log error but continue with interval-based logic
		o.logger.Warnw("error getting last successful export, falling back to interval-based logic",
			"scheduled_task_id", task.ID,
			"error", err)
	}

	// If we found a previous successful export, use its end_time as our start_time (incremental sync)
	if lastTask != nil && lastTask.Metadata != nil {
		if endTimeStr, ok := lastTask.Metadata["end_time"].(string); ok {
			lastEndTime, err := time.Parse(time.RFC3339, endTimeStr)
			if err != nil {
				o.logger.Warnw("failed to parse last task end_time, falling back to interval-based logic",
					"scheduled_task_id", task.ID,
					"end_time_str", endTimeStr,
					"error", err)
			} else {
				o.logger.Infow("using incremental sync for manual sync - starting from last export's end_time",
					"scheduled_task_id", task.ID,
					"last_export_end_time", lastEndTime,
					"new_start_time", lastEndTime,
					"new_end_time", endTime,
					"duration", endTime.Sub(lastEndTime))
				return lastEndTime
			}
		} else {
			o.logger.Warnw("last task metadata missing end_time, falling back to interval-based logic",
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
		startTime = endTime.Add(-24 * time.Hour)
	}

	o.logger.Infow("no previous export found - using interval-based logic for manual sync (first run)",
		"scheduled_task_id", task.ID,
		"interval", interval,
		"start_time", startTime,
		"end_time", endTime,
		"duration", endTime.Sub(startTime))

	return startTime
}
