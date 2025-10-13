package export

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/domain/task"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
)

// TaskActivity handles task-related operations
type TaskActivity struct {
	taskRepo task.Repository
	logger   *logger.Logger
}

// NewTaskActivity creates a new task activity
func NewTaskActivity(taskRepo task.Repository, logger *logger.Logger) *TaskActivity {
	return &TaskActivity{
		taskRepo: taskRepo,
		logger:   logger,
	}
}

// CreateTaskInput represents input for creating a task
type CreateTaskInput struct {
	ScheduledJobID string
	TenantID       string
	EnvID          string
	EntityType     string
	StartTime      time.Time
	EndTime        time.Time
}

// CreateTaskOutput represents output from creating a task
type CreateTaskOutput struct {
	TaskID string
}

// CreateTask creates a new export task
func (a *TaskActivity) CreateTask(ctx context.Context, input CreateTaskInput) (*CreateTaskOutput, error) {
	a.logger.Infow("creating export task",
		"scheduled_job_id", input.ScheduledJobID,
		"entity_type", input.EntityType,
		"start_time", input.StartTime,
		"end_time", input.EndTime)

	// Create task
	now := time.Now()
	newTask := &task.Task{
		ID:             types.GenerateUUIDWithPrefix("task"),
		EnvironmentID:  input.EnvID,
		TaskType:       types.TaskTypeExport,
		EntityType:     types.EntityType(input.EntityType),
		ScheduledJobID: input.ScheduledJobID,
		FileURL:        "", // Will be set after upload
		FileType:       types.FileTypeCSV,
		TaskStatus:     types.TaskStatusPending,
		Metadata: map[string]interface{}{
			"start_time": input.StartTime.Format(time.RFC3339),
			"end_time":   input.EndTime.Format(time.RFC3339),
		},
		StartedAt: &now,
		BaseModel: types.BaseModel{
			TenantID:  input.TenantID,
			Status:    types.StatusPublished,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	err := a.taskRepo.Create(ctx, newTask)
	if err != nil {
		a.logger.Errorw("failed to create task", "error", err)
		return nil, ierr.WithError(err).
			WithHint("Failed to create export task").
			Mark(ierr.ErrDatabase)
	}

	a.logger.Infow("export task created", "task_id", newTask.ID)

	return &CreateTaskOutput{
		TaskID: newTask.ID,
	}, nil
}

// UpdateTaskStatusInput represents input for updating task status
type UpdateTaskStatusInput struct {
	TaskID     string
	Status     types.TaskStatus
	RecordInfo *RecordInfo // Optional: for tracking progress
	Error      string      // Optional: for failures
}

// RecordInfo holds record count information
type RecordInfo struct {
	TotalRecords      int
	ProcessedRecords  int
	SuccessfulRecords int
	FailedRecords     int
}

// UpdateTaskStatus updates the status of a task
func (a *TaskActivity) UpdateTaskStatus(ctx context.Context, input UpdateTaskStatusInput) error {
	a.logger.Infow("updating task status",
		"task_id", input.TaskID,
		"status", input.Status)

	existingTask, err := a.taskRepo.Get(ctx, input.TaskID)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to get task").
			Mark(ierr.ErrDatabase)
	}

	// Update status
	existingTask.TaskStatus = input.Status

	// Update record counts if provided
	if input.RecordInfo != nil {
		existingTask.TotalRecords = &input.RecordInfo.TotalRecords
		existingTask.ProcessedRecords = input.RecordInfo.ProcessedRecords
		existingTask.SuccessfulRecords = input.RecordInfo.SuccessfulRecords
		existingTask.FailedRecords = input.RecordInfo.FailedRecords
	}

	// Update timestamps based on status
	now := time.Now()
	switch input.Status {
	case types.TaskStatusProcessing:
		if existingTask.StartedAt == nil {
			existingTask.StartedAt = &now
		}
	case types.TaskStatusCompleted:
		existingTask.CompletedAt = &now
	case types.TaskStatusFailed:
		existingTask.FailedAt = &now
		if input.Error != "" {
			existingTask.ErrorSummary = &input.Error
		}
	}

	err = a.taskRepo.Update(ctx, existingTask)
	if err != nil {
		a.logger.Errorw("failed to update task status", "error", err)
		return ierr.WithError(err).
			WithHint("Failed to update task status").
			Mark(ierr.ErrDatabase)
	}

	a.logger.Infow("task status updated", "task_id", input.TaskID, "status", input.Status)
	return nil
}

// CompleteTaskInput represents input for completing a task
type CompleteTaskInput struct {
	TaskID      string
	FileURL     string
	RecordCount int
	FileSize    int64
}

// CompleteTask marks a task as completed with final details
func (a *TaskActivity) CompleteTask(ctx context.Context, input CompleteTaskInput) error {
	a.logger.Infow("completing task",
		"task_id", input.TaskID,
		"file_url", input.FileURL,
		"record_count", input.RecordCount)

	existingTask, err := a.taskRepo.Get(ctx, input.TaskID)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to get task").
			Mark(ierr.ErrDatabase)
	}

	// Update task with completion details
	existingTask.TaskStatus = types.TaskStatusCompleted
	existingTask.FileURL = input.FileURL
	existingTask.TotalRecords = &input.RecordCount
	existingTask.SuccessfulRecords = input.RecordCount
	existingTask.ProcessedRecords = input.RecordCount

	now := time.Now()
	existingTask.CompletedAt = &now

	// Add file details to metadata
	if existingTask.Metadata == nil {
		existingTask.Metadata = make(map[string]interface{})
	}
	existingTask.Metadata["file_size_bytes"] = input.FileSize
	existingTask.Metadata["completed_at"] = now.Format(time.RFC3339)

	err = a.taskRepo.Update(ctx, existingTask)
	if err != nil {
		a.logger.Errorw("failed to complete task", "error", err)
		return ierr.WithError(err).
			WithHint("Failed to complete task").
			Mark(ierr.ErrDatabase)
	}

	a.logger.Infow("task completed successfully", "task_id", input.TaskID)
	return nil
}
