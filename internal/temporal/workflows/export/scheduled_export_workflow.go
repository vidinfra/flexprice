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
	ScheduledTaskID string
	TenantID        string
	EnvID           string
}

// ScheduledExportWorkflow is the lightweight cron wrapper
// It fetches the scheduled job config and triggers the actual export workflow
func ScheduledExportWorkflow(ctx workflow.Context, input ScheduledExportWorkflowInput) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting scheduled export workflow",
		"scheduled_task_id", input.ScheduledTaskID,
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

	// Activity to fetch scheduled task and calculate time range
	var scheduledTaskActivity export.ScheduledTaskActivity

	var taskDetails export.ScheduledTaskDetails
	err := workflow.ExecuteActivity(ctx, scheduledTaskActivity.GetScheduledTaskDetails, export.GetScheduledTaskDetailsInput{
		ScheduledTaskID: input.ScheduledTaskID,
		TenantID:        input.TenantID,
		EnvID:           input.EnvID,
	}).Get(ctx, &taskDetails)
	if err != nil {
		logger.Error("Failed to get scheduled task details", "error", err)
		return fmt.Errorf("failed to get scheduled task: %w", err)
	}

	// Check if task is enabled
	if !taskDetails.Enabled {
		logger.Info("Scheduled task is disabled, skipping execution")
		return nil
	}

	logger.Info("Scheduled task details retrieved",
		"entity_type", taskDetails.EntityType,
		"start_time", taskDetails.StartTime,
		"end_time", taskDetails.EndTime)

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
		TaskID:          taskID,
		ScheduledTaskID: input.ScheduledTaskID,
		EntityType:      types.ExportEntityType(taskDetails.EntityType),
		ConnectionID:    taskDetails.ConnectionID,
		TenantID:        taskDetails.TenantID,
		EnvID:           taskDetails.EnvID,
		UserID:          "", // Scheduled runs have no user context
		StartTime:       taskDetails.StartTime,
		EndTime:         taskDetails.EndTime,
		JobConfig:       taskDetails.JobConfig,
	}

	var exportOutput ExecuteExportWorkflowOutput
	err = workflow.ExecuteChildWorkflow(childCtx, ExecuteExportWorkflow, exportInput).Get(ctx, &exportOutput)

	// Always update scheduled task's last run fields regardless of success/failure
	now := time.Now()
	var lastRunStatus string
	var lastRunError string

	if err != nil {
		logger.Error("Export workflow failed", "error", err)
		lastRunStatus = "failed"
		lastRunError = err.Error()
	} else {
		logger.Info("Scheduled export completed successfully",
			"task_id", exportOutput.TaskID,
			"record_count", exportOutput.RecordCount)
		lastRunStatus = "success"
		lastRunError = ""
	}

	// Update scheduled task's last run fields
	updateInput := export.UpdateScheduledTaskInput{
		ScheduledTaskID: input.ScheduledTaskID,
		TenantID:        input.TenantID,
		EnvID:           input.EnvID,
		LastRunAt:       now,
		LastRunStatus:   lastRunStatus,
		LastRunError:    lastRunError,
	}
	err = workflow.ExecuteActivity(ctx, scheduledTaskActivity.UpdateScheduledTaskLastRun, updateInput).Get(ctx, nil)
	if err != nil {
		logger.Error("Failed to update scheduled task last run", "error", err)
		// Continue anyway
	}

	// Return the original error if the export failed
	if lastRunStatus == "failed" {
		return fmt.Errorf("export workflow failed: %w", err)
	}

	return nil
}
