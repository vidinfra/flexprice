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
	TriggerManualSync(ctx context.Context, id string) (string, error)
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

// UpdateScheduledTask updates a scheduled task
func (s *scheduledTaskService) UpdateScheduledTask(ctx context.Context, id string, req dto.UpdateScheduledTaskRequest) (*dto.ScheduledTaskResponse, error) {
	// Get existing task
	task, err := s.repo.Get(ctx, id)
	if err != nil {
		s.logger.Errorw("failed to get scheduled task for update", "id", id, "error", err)
		return nil, ierr.WithError(err).
			WithHint("Scheduled task not found").
			Mark(ierr.ErrNotFound)
	}

	// Track if enabled status changed
	wasEnabled := task.Enabled

	// Update fields if provided
	if req.Interval != nil {
		interval := types.ScheduledTaskInterval(*req.Interval)
		if err := interval.Validate(); err != nil {
			return nil, err
		}
		task.Interval = string(interval)
		// Recalculate next run time
		now := time.Now()
		nextRun := task.CalculateNextRunTime(now)
		task.NextRunAt = &nextRun
	}

	if req.Enabled != nil {
		task.Enabled = *req.Enabled
	}

	if req.JobConfig != nil {
		// Validate new job config
		jobConfigBytes, err := json.Marshal(*req.JobConfig)
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

		if err := s3Config.Validate(); err != nil {
			return nil, err
		}

		task.JobConfig = *req.JobConfig
	}

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

	// Handle Temporal schedule enable/disable
	if s.orchestrator != nil && wasEnabled != task.Enabled {
		if task.Enabled {
			// Start/Unpause the schedule
			err = s.orchestrator.StartScheduledTask(ctx, task.ID)
			if err != nil {
				s.logger.Errorw("failed to start temporal schedule", "error", err)
			}
		} else {
			// Pause the schedule
			err = s.orchestrator.StopScheduledTask(ctx, task.ID)
			if err != nil {
				s.logger.Errorw("failed to stop temporal schedule", "error", err)
			}
		}
	}

	s.logger.Infow("scheduled task updated successfully", "id", task.ID)

	return dto.ToScheduledTaskResponse(task), nil
}

// DeleteScheduledTask deletes a scheduled task
func (s *scheduledTaskService) DeleteScheduledTask(ctx context.Context, id string) error {
	err := s.repo.Delete(ctx, id)
	if err != nil {
		s.logger.Errorw("failed to delete scheduled task", "id", id, "error", err)
		return ierr.WithError(err).
			WithHint("Failed to delete scheduled task").
			Mark(ierr.ErrDatabase)
	}

	s.logger.Infow("scheduled task deleted successfully", "id", id)
	return nil
}

// TriggerManualSync triggers a manual export immediately
func (s *scheduledTaskService) TriggerManualSync(ctx context.Context, id string) (string, error) {
	if s.orchestrator == nil {
		return "", ierr.NewError("orchestrator not configured").
			WithHint("Temporal orchestrator is not available").
			Mark(ierr.ErrInternal)
	}

	workflowID, err := s.orchestrator.TriggerManualSync(ctx, id)
	if err != nil {
		s.logger.Errorw("failed to trigger manual sync", "id", id, "error", err)
		return "", err
	}

	s.logger.Infow("manual sync triggered", "id", id, "workflow_id", workflowID)
	return workflowID, nil
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
