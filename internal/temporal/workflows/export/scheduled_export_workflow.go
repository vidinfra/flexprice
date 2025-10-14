package export

import (
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/temporal/activities/export"
	"github.com/flexprice/flexprice/internal/types"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// ScheduledExportWorkflowInput represents input for the scheduled workflow
type ScheduledExportWorkflowInput struct {
	ScheduledJobID string
	TenantID       string
	EnvID          string
}

// ScheduledExportWorkflow is the lightweight cron wrapper
// It fetches the scheduled job config and triggers the actual export workflow
func ScheduledExportWorkflow(ctx workflow.Context, input ScheduledExportWorkflowInput) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting scheduled export workflow",
		"scheduled_job_id", input.ScheduledJobID,
		"tenant_id", input.TenantID,
		"env_id", input.EnvID)

	// Activity options for lightweight operations
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 1 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 2,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	// Activity to fetch scheduled job and calculate time range
	var scheduledJobActivity export.ScheduledJobActivity

	var jobDetails export.ScheduledJobDetails
	err := workflow.ExecuteActivity(ctx, scheduledJobActivity.GetScheduledJobDetails, export.GetScheduledJobDetailsInput{
		ScheduledJobID: input.ScheduledJobID,
		TenantID:       input.TenantID,
		EnvID:          input.EnvID,
	}).Get(ctx, &jobDetails)
	if err != nil {
		logger.Error("Failed to get scheduled job details", "error", err)
		return fmt.Errorf("failed to get scheduled job: %w", err)
	}

	// Check if job is enabled
	if !jobDetails.Enabled {
		logger.Info("Scheduled job is disabled, skipping execution")
		return nil
	}

	logger.Info("Scheduled job details retrieved",
		"entity_type", jobDetails.EntityType,
		"start_time", jobDetails.StartTime,
		"end_time", jobDetails.EndTime)

	// Generate task ID and use it as workflow ID
	// Format: {task_id}-export
	var taskID string
	workflow.SideEffect(ctx, func(ctx workflow.Context) interface{} {
		return types.GenerateUUIDWithPrefix("task")
	}).Get(&taskID)
	workflowID := fmt.Sprintf("%s-export", taskID)

	logger.Info("Generated task ID for workflow", "task_id", taskID, "workflow_id", workflowID)

	// Execute the actual export workflow
	childWorkflowOptions := workflow.ChildWorkflowOptions{
		WorkflowID: workflowID,
	}
	childCtx := workflow.WithChildOptions(ctx, childWorkflowOptions)

	exportInput := ExecuteExportWorkflowInput{
		TaskID:         taskID,
		ScheduledJobID: input.ScheduledJobID,
		EntityType:     types.ExportEntityType(jobDetails.EntityType),
		ConnectionID:   jobDetails.ConnectionID,
		TenantID:       jobDetails.TenantID,
		EnvID:          jobDetails.EnvID,
		StartTime:      jobDetails.StartTime,
		EndTime:        jobDetails.EndTime,
		JobConfig:      jobDetails.JobConfig,
	}

	var exportOutput ExecuteExportWorkflowOutput
	err = workflow.ExecuteChildWorkflow(childCtx, ExecuteExportWorkflow, exportInput).Get(ctx, &exportOutput)
	if err != nil {
		logger.Error("Export workflow failed", "error", err)
		return fmt.Errorf("export workflow failed: %w", err)
	}

	logger.Info("Scheduled export completed successfully",
		"task_id", exportOutput.TaskID,
		"record_count", exportOutput.RecordCount)

	// Update scheduled job's last_run_at
	updateInput := export.UpdateScheduledJobInput{
		ScheduledJobID: input.ScheduledJobID,
		TenantID:       input.TenantID,
		EnvID:          input.EnvID,
		LastRunAt:      time.Now(),
		LastRunStatus:  "success",
	}
	err = workflow.ExecuteActivity(ctx, scheduledJobActivity.UpdateScheduledJobLastRun, updateInput).Get(ctx, nil)
	if err != nil {
		logger.Error("Failed to update scheduled job last run", "error", err)
		// Continue anyway
	}

	return nil
}
