package service

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/domain/scheduledjob"
	"github.com/flexprice/flexprice/internal/domain/task"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	exportWorkflows "github.com/flexprice/flexprice/internal/temporal/workflows/export"
	"github.com/flexprice/flexprice/internal/types"
	"go.temporal.io/sdk/client"
)

// ScheduledJobOrchestrator manages Temporal schedules for scheduled jobs
type ScheduledJobOrchestrator struct {
	scheduledJobRepo scheduledjob.Repository
	taskRepo         task.Repository
	temporalClient   client.Client
	logger           *logger.Logger
}

// NewScheduledJobOrchestrator creates a new orchestrator
func NewScheduledJobOrchestrator(
	scheduledJobRepo scheduledjob.Repository,
	taskRepo task.Repository,
	temporalClient client.Client,
	logger *logger.Logger,
) *ScheduledJobOrchestrator {
	return &ScheduledJobOrchestrator{
		scheduledJobRepo: scheduledJobRepo,
		taskRepo:         taskRepo,
		temporalClient:   temporalClient,
		logger:           logger,
	}
}

// StartScheduledJob creates and starts a Temporal schedule for the job
func (o *ScheduledJobOrchestrator) StartScheduledJob(ctx context.Context, jobID string) error {
	o.logger.Infow("starting scheduled job", "job_id", jobID)

	// Get the job
	job, err := o.scheduledJobRepo.Get(ctx, jobID)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to get scheduled job").
			Mark(ierr.ErrDatabase)
	}

	// If already has a schedule, unpause it
	if job.TemporalScheduleID != "" {
		o.logger.Infow("temporal schedule already exists, unpausing",
			"schedule_id", job.TemporalScheduleID)

		handle := o.temporalClient.ScheduleClient().GetHandle(ctx, job.TemporalScheduleID)
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
	// Use the scheduled job ID directly (already has schdjob_ prefix)
	scheduleID := job.ID
	cronExpr := o.getCronExpression(types.ScheduledJobInterval(job.Interval))

	scheduleSpec := client.ScheduleSpec{
		CronExpressions: []string{cronExpr},
	}

	action := &client.ScheduleWorkflowAction{
		// Don't set ID here - let Temporal auto-generate for the wrapper workflow
		// The child workflow (ExecuteExportWorkflow) will use task_id-export format
		Workflow: exportWorkflows.ScheduledExportWorkflow,
		Args: []interface{}{
			exportWorkflows.ScheduledExportWorkflowInput{
				ScheduledJobID: job.ID,
				TenantID:       job.TenantID,
				EnvID:          job.EnvironmentID,
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

	// Update job with schedule ID
	job.TemporalScheduleID = scheduleID
	err = o.scheduledJobRepo.Update(ctx, job)
	if err != nil {
		o.logger.Errorw("failed to update job with schedule ID", "error", err)
		// Try to delete the created schedule
		_ = schedule.Delete(ctx)
		return ierr.WithError(err).
			WithHint("Failed to save schedule ID").
			Mark(ierr.ErrDatabase)
	}

	o.logger.Infow("scheduled job started successfully", "job_id", jobID)
	return nil
}

// StopScheduledJob pauses the Temporal schedule
func (o *ScheduledJobOrchestrator) StopScheduledJob(ctx context.Context, jobID string) error {
	o.logger.Infow("stopping scheduled job", "job_id", jobID)

	job, err := o.scheduledJobRepo.Get(ctx, jobID)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to get scheduled job").
			Mark(ierr.ErrDatabase)
	}

	if job.TemporalScheduleID == "" {
		o.logger.Infow("no temporal schedule to stop", "job_id", jobID)
		return nil
	}

	// Pause the schedule
	handle := o.temporalClient.ScheduleClient().GetHandle(ctx, job.TemporalScheduleID)
	err = handle.Pause(ctx, client.SchedulePauseOptions{})
	if err != nil {
		o.logger.Errorw("failed to pause schedule", "error", err)
		return ierr.WithError(err).
			WithHint("Failed to pause Temporal schedule").
			Mark(ierr.ErrInternal)
	}

	o.logger.Infow("scheduled job stopped successfully", "job_id", jobID)
	return nil
}

// TriggerManualSync executes the export workflow immediately (bypasses schedule)
func (o *ScheduledJobOrchestrator) TriggerManualSync(ctx context.Context, jobID string) (string, error) {
	o.logger.Infow("triggering manual sync", "job_id", jobID)

	// Get the job
	job, err := o.scheduledJobRepo.Get(ctx, jobID)
	if err != nil {
		return "", ierr.WithError(err).
			WithHint("Failed to get scheduled job").
			Mark(ierr.ErrDatabase)
	}

	// Get job config
	jobConfig, err := job.GetS3JobConfig()
	if err != nil {
		return "", ierr.WithError(err).
			WithHint("Invalid job configuration").
			Mark(ierr.ErrValidation)
	}

	// Calculate time range
	endTime := time.Now()
	startTime := o.calculateManualSyncStartTime(ctx, job, endTime)

	// Generate task ID and use it as workflow ID
	// Format: {task_id}-export
	taskID := types.GenerateUUIDWithPrefix("task")
	workflowID := fmt.Sprintf("%s-export", taskID)

	o.logger.Infow("triggering manual export",
		"task_id", taskID,
		"workflow_id", workflowID,
		"job_id", jobID)

	workflowOptions := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: string(types.TemporalTaskQueueExport),
	}

	input := exportWorkflows.ExecuteExportWorkflowInput{
		TaskID:         taskID,
		ScheduledJobID: job.ID,
		EntityType:     types.ExportEntityType(job.EntityType),
		ConnectionID:   job.ConnectionID,
		TenantID:       job.TenantID,
		EnvID:          job.EnvironmentID,
		StartTime:      startTime,
		EndTime:        endTime,
		JobConfig:      jobConfig,
	}

	workflowRun, err := o.temporalClient.ExecuteWorkflow(ctx, workflowOptions, exportWorkflows.ExecuteExportWorkflow, input)
	if err != nil {
		o.logger.Errorw("failed to start export workflow", "error", err)
		return "", ierr.WithError(err).
			WithHint("Failed to start export workflow").
			Mark(ierr.ErrInternal)
	}

	o.logger.Infow("manual sync triggered",
		"job_id", jobID,
		"workflow_id", workflowRun.GetID(),
		"run_id", workflowRun.GetRunID())

	return workflowRun.GetID(), nil
}

// getCronExpression converts interval to cron expression
func (o *ScheduledJobOrchestrator) getCronExpression(interval types.ScheduledJobInterval) string {
	// HARDCODED FOR TESTING: Run every 3 minutes
	return "*/3 * * * *" // Every 3 minutes

	// Original logic (commented out for testing)
	// switch interval {
	// case types.ScheduledJobIntervalHourly:
	// 	return "0 * * * *" // Every hour
	// case types.ScheduledJobIntervalDaily:
	// 	return "0 0 * * *" // Every day at midnight
	// case types.ScheduledJobIntervalWeekly:
	// 	return "0 0 * * 0" // Every Sunday at midnight
	// case types.ScheduledJobIntervalMonthly:
	// 	return "0 0 1 * *" // First day of every month at midnight
	// default:
	// 	return "0 0 * * *" // Default to daily
	// }
}

// calculateManualSyncStartTime determines start time for manual sync
// Implements incremental sync: uses last successful export's end_time as new start_time
func (o *ScheduledJobOrchestrator) calculateManualSyncStartTime(ctx context.Context, job *scheduledjob.ScheduledJob, endTime time.Time) time.Time {
	o.logger.Infow("calculating start time for manual sync",
		"scheduled_job_id", job.ID,
		"interval", job.Interval,
		"end_time", endTime)

	// Try to get last successful export task for incremental sync
	lastTask, err := o.taskRepo.GetLastSuccessfulExportTask(ctx, job.ID)
	if err != nil {
		// Log error but continue with interval-based logic
		o.logger.Warnw("error getting last successful export, falling back to interval-based logic",
			"scheduled_job_id", job.ID,
			"error", err)
	}

	// If we found a previous successful export, use its end_time as our start_time (incremental sync)
	if lastTask != nil && lastTask.Metadata != nil {
		if endTimeStr, ok := lastTask.Metadata["end_time"].(string); ok {
			lastEndTime, err := time.Parse(time.RFC3339, endTimeStr)
			if err != nil {
				o.logger.Warnw("failed to parse last task end_time, falling back to interval-based logic",
					"scheduled_job_id", job.ID,
					"end_time_str", endTimeStr,
					"error", err)
			} else {
				o.logger.Infow("using incremental sync for manual sync - starting from last export's end_time",
					"scheduled_job_id", job.ID,
					"last_export_end_time", lastEndTime,
					"new_start_time", lastEndTime,
					"new_end_time", endTime,
					"duration", endTime.Sub(lastEndTime))
				return lastEndTime
			}
		} else {
			o.logger.Warnw("last task metadata missing end_time, falling back to interval-based logic",
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
		startTime = endTime.Add(-24 * time.Hour)
	}

	o.logger.Infow("no previous export found - using interval-based logic for manual sync (first run)",
		"scheduled_job_id", job.ID,
		"interval", interval,
		"start_time", startTime,
		"end_time", endTime,
		"duration", endTime.Sub(startTime))

	return startTime
}
