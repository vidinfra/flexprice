package service

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/domain/scheduledjob"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	exportWorkflows "github.com/flexprice/flexprice/internal/temporal/workflows/export"
	"github.com/flexprice/flexprice/internal/types"
	"go.temporal.io/sdk/client"
)

// ScheduledJobOrchestrator manages Temporal schedules for scheduled jobs
type ScheduledJobOrchestrator struct {
	scheduledJobRepo scheduledjob.Repository
	temporalClient   client.Client
	logger           *logger.Logger
}

// NewScheduledJobOrchestrator creates a new orchestrator
func NewScheduledJobOrchestrator(
	scheduledJobRepo scheduledjob.Repository,
	temporalClient client.Client,
	logger *logger.Logger,
) *ScheduledJobOrchestrator {
	return &ScheduledJobOrchestrator{
		scheduledJobRepo: scheduledJobRepo,
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
	scheduleID := fmt.Sprintf("schdjob_%s", job.ID)
	cronExpr := o.getCronExpression(types.ScheduledJobInterval(job.Interval))

	scheduleSpec := client.ScheduleSpec{
		CronExpressions: []string{cronExpr},
	}

	action := &client.ScheduleWorkflowAction{
		ID:       scheduleID + "-workflow",
		Workflow: exportWorkflows.ScheduledExportWorkflow,
		Args: []interface{}{
			exportWorkflows.ScheduledExportWorkflowInput{
				ScheduledJobID: job.ID,
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
	startTime := o.calculateManualSyncStartTime(job, endTime)

	// Execute workflow directly (not through schedule)
	workflowID := fmt.Sprintf("manual-export-%s-%d", jobID, time.Now().Unix())

	workflowOptions := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: string(types.TemporalTaskQueueExport),
	}

	input := exportWorkflows.ExecuteExportWorkflowInput{
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
	switch interval {
	case types.ScheduledJobIntervalHourly:
		return "0 * * * *" // Every hour
	case types.ScheduledJobIntervalDaily:
		return "0 0 * * *" // Every day at midnight
	case types.ScheduledJobIntervalWeekly:
		return "0 0 * * 0" // Every Sunday at midnight
	case types.ScheduledJobIntervalMonthly:
		return "0 0 1 * *" // First day of every month at midnight
	default:
		return "0 0 * * *" // Default to daily
	}
}

// calculateManualSyncStartTime determines start time for manual sync
func (o *ScheduledJobOrchestrator) calculateManualSyncStartTime(job *scheduledjob.ScheduledJob, endTime time.Time) time.Time {
	// Use the same logic as scheduled runs
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
		return endTime.Add(-24 * time.Hour)
	}
}
