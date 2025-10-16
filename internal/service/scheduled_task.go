package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/scheduledtask"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
)

// ScheduledTaskService handles scheduled task operations
type ScheduledTaskService interface {
	CreateScheduledTask(ctx context.Context, req dto.CreateScheduledTaskRequest) (*dto.ScheduledTaskResponse, error)
	GetScheduledTask(ctx context.Context, id string) (*dto.ScheduledTaskResponse, error)
	ListScheduledTasks(ctx context.Context, filter *types.QueryFilter) (*dto.ListScheduledTasksResponse, error)
	UpdateScheduledTask(ctx context.Context, id string, req dto.UpdateScheduledTaskRequest) (*dto.ScheduledTaskResponse, error)
	DeleteScheduledTask(ctx context.Context, id string) error
	GetScheduledTasksByEntityType(ctx context.Context, entityType types.ScheduledTaskEntityType) ([]*dto.ScheduledTaskResponse, error)
	TriggerForceRun(ctx context.Context, id string, req dto.TriggerForceRunRequest) (*dto.TriggerForceRunResponse, error)
}

type scheduledTaskService struct {
	repo         scheduledtask.Repository
	orchestrator *ScheduledTaskOrchestrator
	logger       *logger.Logger
}

// NewScheduledTaskService creates a new scheduled task service
func NewScheduledTaskService(
	repo scheduledtask.Repository,
	orchestrator *ScheduledTaskOrchestrator,
	logger *logger.Logger,
) ScheduledTaskService {
	return &scheduledTaskService{
		repo:         repo,
		orchestrator: orchestrator,
		logger:       logger,
	}
}

// CreateScheduledTask creates a new scheduled task
func (s *scheduledTaskService) CreateScheduledTask(ctx context.Context, req dto.CreateScheduledTaskRequest) (*dto.ScheduledTaskResponse, error) {
	// Validate entity type
	entityType := types.ScheduledTaskEntityType(req.EntityType)
	if err := entityType.Validate(); err != nil {
		return nil, err
	}

	// Validate interval
	interval := types.ScheduledTaskInterval(req.Interval)
	if err := interval.Validate(); err != nil {
		return nil, err
	}

	// Parse and validate job config as S3JobConfig
	jobConfigBytes, err := json.Marshal(req.JobConfig)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Invalid job configuration format").
			Mark(ierr.ErrValidation)
	}

	var s3Config types.S3JobConfig
	if err := json.Unmarshal(jobConfigBytes, &s3Config); err != nil {
		return nil, ierr.WithError(err).
			WithHint("Invalid S3 job configuration format").
			Mark(ierr.ErrValidation)
	}

	// Validate S3 config
	if err := s3Config.Validate(); err != nil {
		return nil, err
	}

	// Create scheduled task
	now := time.Now()

	tenantID := types.GetTenantID(ctx)
	envID := types.GetEnvironmentID(ctx)

	s.logger.Infow("creating scheduled task",
		"tenant_id", tenantID,
		"environment_id", envID,
		"entity_type", entityType)

	task := &scheduledtask.ScheduledTask{
		ID:            types.GenerateUUIDWithPrefix("schtask"),
		TenantID:      tenantID,
		EnvironmentID: envID,
		ConnectionID:  req.ConnectionID,
		EntityType:    string(entityType),
		Interval:      string(interval),
		Enabled:       req.Enabled,
		JobConfig:     req.JobConfig,
		Status:        "published",
		CreatedAt:     now,
		UpdatedAt:     now,
		CreatedBy:     types.GetUserID(ctx),
		UpdatedBy:     types.GetUserID(ctx),
	}

	// Calculate next run time
	nextRun := task.CalculateNextRunTime(now)
	task.NextRunAt = &nextRun

	// Save to database
	err = s.repo.Create(ctx, task)
	if err != nil {
		s.logger.Errorw("failed to create scheduled task", "error", err)
		return nil, ierr.WithError(err).
			WithHint("Failed to create scheduled task").
			Mark(ierr.ErrDatabase)
	}

	s.logger.Infow("scheduled task created successfully",
		"id", task.ID,
		"entity_type", task.EntityType,
		"interval", task.Interval)

	// Start Temporal schedule if enabled
	if task.Enabled && s.orchestrator != nil {
		err = s.orchestrator.StartScheduledTask(ctx, task.ID)
		if err != nil {
			s.logger.Errorw("failed to start temporal schedule", "error", err)
			// Don't fail the creation - task is created, schedule can be started later
		}
	}

	return dto.ToScheduledTaskResponse(task), nil
}

// GetScheduledTask retrieves a scheduled task by ID
func (s *scheduledTaskService) GetScheduledTask(ctx context.Context, id string) (*dto.ScheduledTaskResponse, error) {
	task, err := s.repo.Get(ctx, id)
	if err != nil {
		s.logger.Errorw("failed to get scheduled task", "id", id, "error", err)
		return nil, ierr.WithError(err).
			WithHint("Scheduled task not found").
			Mark(ierr.ErrNotFound)
	}

	return dto.ToScheduledTaskResponse(task), nil
}

// ListScheduledTasks retrieves a list of scheduled tasks
func (s *scheduledTaskService) ListScheduledTasks(ctx context.Context, filter *types.QueryFilter) (*dto.ListScheduledTasksResponse, error) {
	if filter == nil {
		filter = types.NewDefaultQueryFilter()
	}

	// Convert QueryFilter to ListFilters
	listFilters := &scheduledtask.ListFilters{
		Limit:  int(*filter.Limit),
		Offset: int(*filter.Offset),
	}

	tasks, err := s.repo.List(ctx, listFilters)
	if err != nil {
		s.logger.Errorw("failed to list scheduled tasks", "error", err)
		return nil, ierr.WithError(err).
			WithHint("Failed to retrieve scheduled tasks").
			Mark(ierr.ErrDatabase)
	}

	// Get total count (for pagination)
	totalCount := len(tasks) // TODO: implement proper count query if needed

	return dto.ToScheduledTaskListResponse(tasks, totalCount), nil
}

// UpdateScheduledTask updates a scheduled task (only enabled field can be changed)
func (s *scheduledTaskService) UpdateScheduledTask(ctx context.Context, id string, req dto.UpdateScheduledTaskRequest) (*dto.ScheduledTaskResponse, error) {
	// Validate request
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Get existing task
	task, err := s.repo.Get(ctx, id)
	if err != nil {
		s.logger.Errorw("failed to get scheduled task for update", "id", id, "error", err)
		return nil, ierr.WithError(err).
			WithHint("Scheduled task not found").
			Mark(ierr.ErrNotFound)
	}

	// Check if task is archived
	if task.Status == string(types.StatusArchived) {
		return nil, ierr.NewError("cannot update archived scheduled task").
			WithHint("This scheduled task has been archived and cannot be modified").
			WithReportableDetails(map[string]interface{}{
				"task_id": id,
				"status":  task.Status,
			}).
			Mark(ierr.ErrValidation)
	}

	// Track if enabled status changed
	wasEnabled := task.Enabled
	newEnabled := *req.Enabled

	// Check if there's actually a change
	if wasEnabled == newEnabled {
		s.logger.Infow("no change in enabled status", "id", id, "enabled", newEnabled)
		return dto.ToScheduledTaskResponse(task), nil
	}

	// Update enabled status
	task.Enabled = newEnabled
	task.UpdatedAt = time.Now()
	task.UpdatedBy = types.GetUserID(ctx)

	// Save updated task
	err = s.repo.Update(ctx, task)
	if err != nil {
		s.logger.Errorw("failed to update scheduled task", "id", id, "error", err)
		return nil, ierr.WithError(err).
			WithHint("Failed to update scheduled task").
			Mark(ierr.ErrDatabase)
	}

	// Handle Temporal schedule pause/resume
	if s.orchestrator != nil {
		if newEnabled {
			// Resume/Start the schedule
			s.logger.Infow("resuming temporal schedule", "task_id", id)
			err = s.orchestrator.StartScheduledTask(ctx, task.ID)
			if err != nil {
				s.logger.Errorw("failed to start temporal schedule", "task_id", id, "error", err)
				// Rollback the database change
				task.Enabled = wasEnabled
				_ = s.repo.Update(ctx, task)
				return nil, ierr.WithError(err).
					WithHint("Failed to resume schedule in Temporal").
					Mark(ierr.ErrInternal)
			}
			s.logger.Infow("temporal schedule resumed successfully", "task_id", id)
		} else {
			// Pause the schedule
			s.logger.Infow("pausing temporal schedule", "task_id", id)
			err = s.orchestrator.StopScheduledTask(ctx, task.ID)
			if err != nil {
				s.logger.Errorw("failed to stop temporal schedule", "task_id", id, "error", err)
				// Rollback the database change
				task.Enabled = wasEnabled
				_ = s.repo.Update(ctx, task)
				return nil, ierr.WithError(err).
					WithHint("Failed to pause schedule in Temporal").
					Mark(ierr.ErrInternal)
			}
			s.logger.Infow("temporal schedule paused successfully", "task_id", id)
		}
	}

	action := "paused"
	if newEnabled {
		action = "resumed"
	}
	s.logger.Infow("scheduled task updated successfully",
		"id", task.ID,
		"action", action,
		"enabled", newEnabled)

	return dto.ToScheduledTaskResponse(task), nil
}

// DeleteScheduledTask archives a scheduled task (soft delete)
func (s *scheduledTaskService) DeleteScheduledTask(ctx context.Context, id string) error {
	// Get existing task
	task, err := s.repo.Get(ctx, id)
	if err != nil {
		s.logger.Errorw("failed to get scheduled task for deletion", "id", id, "error", err)
		return ierr.WithError(err).
			WithHint("Scheduled task not found").
			Mark(ierr.ErrNotFound)
	}

	// Check if already archived
	if task.Status == string(types.StatusArchived) {
		s.logger.Infow("scheduled task already archived", "id", id)
		return ierr.NewError("scheduled task is already archived").
			WithHint("This scheduled task has already been deleted").
			WithReportableDetails(map[string]interface{}{
				"task_id": id,
				"status":  task.Status,
			}).
			Mark(ierr.ErrValidation)
	}

	// Archive the task (soft delete)
	task.Status = string(types.StatusArchived)
	task.Enabled = false // Disable when archiving
	task.UpdatedAt = time.Now()
	task.UpdatedBy = types.GetUserID(ctx)

	// Save the archived task
	err = s.repo.Update(ctx, task)
	if err != nil {
		s.logger.Errorw("failed to archive scheduled task", "id", id, "error", err)
		return ierr.WithError(err).
			WithHint("Failed to archive scheduled task").
			Mark(ierr.ErrDatabase)
	}

	// Delete the Temporal schedule
	if s.orchestrator != nil {
		s.logger.Infow("deleting temporal schedule", "task_id", id)
		err = s.orchestrator.DeleteScheduledTask(ctx, id)
		if err != nil {
			s.logger.Errorw("failed to delete temporal schedule", "task_id", id, "error", err)
			// Don't rollback - task is archived in DB, Temporal cleanup can be retried
			// Log error but continue
			s.logger.Warnw("scheduled task archived in database but temporal cleanup failed - may need manual cleanup",
				"task_id", id,
				"error", err)
		} else {
			s.logger.Infow("temporal schedule deleted successfully", "task_id", id)
		}
	}

	s.logger.Infow("scheduled task archived successfully",
		"id", id,
		"status", task.Status)
	return nil
}

// TriggerForceRun triggers a force run export immediately with optional custom time range
func (s *scheduledTaskService) TriggerForceRun(ctx context.Context, id string, req dto.TriggerForceRunRequest) (*dto.TriggerForceRunResponse, error) {
	if s.orchestrator == nil {
		return nil, ierr.NewError("orchestrator not configured").
			WithHint("Temporal orchestrator is not available").
			Mark(ierr.ErrInternal)
	}

	workflowID, startTime, endTime, mode, err := s.orchestrator.TriggerForceRun(ctx, id, req.StartTime, req.EndTime)
	if err != nil {
		s.logger.Errorw("failed to trigger force run", "id", id, "error", err)
		return nil, err
	}

	s.logger.Infow("force run triggered",
		"id", id,
		"workflow_id", workflowID,
		"start_time", startTime,
		"end_time", endTime,
		"mode", mode)

	return &dto.TriggerForceRunResponse{
		WorkflowID: workflowID,
		Message:    "Force run triggered successfully",
		StartTime:  startTime,
		EndTime:    endTime,
		Mode:       mode,
	}, nil
}

// GetScheduledTasksByEntityType retrieves scheduled tasks by entity type
func (s *scheduledTaskService) GetScheduledTasksByEntityType(ctx context.Context, entityType types.ScheduledTaskEntityType) ([]*dto.ScheduledTaskResponse, error) {
	tasks, err := s.repo.GetByEntityType(ctx, string(entityType))
	if err != nil {
		s.logger.Errorw("failed to get scheduled tasks by entity type", "entity_type", entityType, "error", err)
		return nil, ierr.WithError(err).
			WithHint("Failed to retrieve scheduled tasks").
			Mark(ierr.ErrDatabase)
	}

	responses := make([]*dto.ScheduledTaskResponse, 0, len(tasks))
	for _, task := range tasks {
		responses = append(responses, dto.ToScheduledTaskResponse(task))
	}

	return responses, nil
}
