package export

import (
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/temporal/activities/export"
	"github.com/flexprice/flexprice/internal/types"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// ExecuteExportWorkflowInput represents input for the export workflow
type ExecuteExportWorkflowInput struct {
	TaskID         string // Pre-generated task ID (used as workflow ID)
	ScheduledJobID string
	EntityType     types.ExportEntityType
	ConnectionID   string
	TenantID       string
	EnvID          string
	StartTime      time.Time
	EndTime        time.Time
	JobConfig      *types.S3JobConfig
}

// ExecuteExportWorkflowOutput represents output from the export workflow
type ExecuteExportWorkflowOutput struct {
	TaskID      string
	FileURL     string
	RecordCount int
	Success     bool
}

// ExecuteExportWorkflow performs the actual data export
// This workflow can be called by the scheduled workflow OR triggered manually
func ExecuteExportWorkflow(ctx workflow.Context, input ExecuteExportWorkflowInput) (*ExecuteExportWorkflowOutput, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting export workflow",
		"scheduled_job_id", input.ScheduledJobID,
		"entity_type", input.EntityType,
		"start_time", input.StartTime,
		"end_time", input.EndTime)

	// Activity options
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Minute, // Max time for each activity
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	var taskActivity export.TaskActivity
	var exportActivity export.ExportActivity

	// Step 1: Create task
	// Use pre-generated task ID from input
	taskID := input.TaskID
	workflowID := workflow.GetInfo(ctx).WorkflowExecution.ID
	logger.Info("Starting export workflow", "task_id", taskID, "workflow_id", workflowID)

	logger.Info("Step 1: Creating task in database")
	createTaskInput := export.CreateTaskInput{
		TaskID:         taskID, // Use pre-generated ID
		WorkflowID:     workflowID,
		ScheduledJobID: input.ScheduledJobID,
		TenantID:       input.TenantID,
		EnvID:          input.EnvID,
		EntityType:     string(input.EntityType),
		StartTime:      input.StartTime,
		EndTime:        input.EndTime,
	}

	var createTaskOutput export.CreateTaskOutput
	err := workflow.ExecuteActivity(ctx, taskActivity.CreateTask, createTaskInput).Get(ctx, &createTaskOutput)
	if err != nil {
		logger.Error("Failed to create task", "error", err)
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	logger.Info("Task created in database", "task_id", taskID)

	// Step 2: Update task to processing
	logger.Info("Step 2: Updating task to processing")
	updateStatusInput := export.UpdateTaskStatusInput{
		TaskID:   taskID,
		TenantID: input.TenantID,
		EnvID:    input.EnvID,
		Status:   types.TaskStatusProcessing,
	}
	err = workflow.ExecuteActivity(ctx, taskActivity.UpdateTaskStatus, updateStatusInput).Get(ctx, nil)
	if err != nil {
		logger.Error("Failed to update task status", "error", err)
		// Continue anyway - task creation succeeded
	}

	// Step 3: Export data (prepare + CSV + S3 upload)
	logger.Info("Step 3: Exporting data")
	exportInput := export.ExportDataInput{
		EntityType:   input.EntityType,
		ConnectionID: input.ConnectionID,
		TenantID:     input.TenantID,
		EnvID:        input.EnvID,
		StartTime:    input.StartTime,
		EndTime:      input.EndTime,
		JobConfig:    input.JobConfig,
	}

	var exportOutput export.ExportDataOutput
	err = workflow.ExecuteActivity(ctx, exportActivity.ExportData, exportInput).Get(ctx, &exportOutput)
	if err != nil {
		logger.Error("Export failed", "error", err)

		// Mark task as failed
		failInput := export.UpdateTaskStatusInput{
			TaskID:   taskID,
			TenantID: input.TenantID,
			EnvID:    input.EnvID,
			Status:   types.TaskStatusFailed,
			Error:    err.Error(),
		}
		_ = workflow.ExecuteActivity(ctx, taskActivity.UpdateTaskStatus, failInput).Get(ctx, nil)

		return &ExecuteExportWorkflowOutput{
			TaskID:  taskID,
			Success: false,
		}, fmt.Errorf("export failed: %w", err)
	}

	logger.Info("Export completed",
		"file_url", exportOutput.FileURL,
		"record_count", exportOutput.RecordCount)

	// Step 4: Complete task
	logger.Info("Step 4: Completing task")
	completeInput := export.CompleteTaskInput{
		TaskID:      taskID,
		TenantID:    input.TenantID,
		EnvID:       input.EnvID,
		FileURL:     exportOutput.FileURL,
		RecordCount: exportOutput.RecordCount,
		FileSize:    exportOutput.FileSizeBytes,
	}

	err = workflow.ExecuteActivity(ctx, taskActivity.CompleteTask, completeInput).Get(ctx, nil)
	if err != nil {
		logger.Error("Failed to complete task", "error", err)
		// Continue anyway - export succeeded
	}

	logger.Info("Export workflow completed successfully", "task_id", taskID)

	return &ExecuteExportWorkflowOutput{
		TaskID:      taskID,
		FileURL:     exportOutput.FileURL,
		RecordCount: exportOutput.RecordCount,
		Success:     true,
	}, nil
}
