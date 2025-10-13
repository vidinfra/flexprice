package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/scheduledjob"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
)

// ScheduledJobService handles scheduled job operations
type ScheduledJobService interface {
	CreateScheduledJob(ctx context.Context, req dto.CreateScheduledJobRequest) (*dto.ScheduledJobResponse, error)
	GetScheduledJob(ctx context.Context, id string) (*dto.ScheduledJobResponse, error)
	ListScheduledJobs(ctx context.Context, filter *types.QueryFilter) (*dto.ListScheduledJobsResponse, error)
	UpdateScheduledJob(ctx context.Context, id string, req dto.UpdateScheduledJobRequest) (*dto.ScheduledJobResponse, error)
	DeleteScheduledJob(ctx context.Context, id string) error
	GetScheduledJobsByEntityType(ctx context.Context, entityType types.ScheduledJobEntityType) ([]*dto.ScheduledJobResponse, error)
	TriggerManualSync(ctx context.Context, id string) (string, error)
}

type scheduledJobService struct {
	repo         scheduledjob.Repository
	orchestrator *ScheduledJobOrchestrator
	logger       *logger.Logger
}

// NewScheduledJobService creates a new scheduled job service
func NewScheduledJobService(
	repo scheduledjob.Repository,
	orchestrator *ScheduledJobOrchestrator,
	logger *logger.Logger,
) ScheduledJobService {
	return &scheduledJobService{
		repo:         repo,
		orchestrator: orchestrator,
		logger:       logger,
	}
}

// CreateScheduledJob creates a new scheduled job
func (s *scheduledJobService) CreateScheduledJob(ctx context.Context, req dto.CreateScheduledJobRequest) (*dto.ScheduledJobResponse, error) {
	// Validate entity type
	entityType := types.ScheduledJobEntityType(req.EntityType)
	if err := entityType.Validate(); err != nil {
		return nil, err
	}

	// Validate interval
	interval := types.ScheduledJobInterval(req.Interval)
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

	// Create scheduled job
	now := time.Now()

	tenantID := types.GetTenantID(ctx)
	envID := types.GetEnvironmentID(ctx)

	s.logger.Infow("creating scheduled job",
		"tenant_id", tenantID,
		"environment_id", envID,
		"entity_type", entityType)

	job := &scheduledjob.ScheduledJob{
		ID:            types.GenerateUUIDWithPrefix("schdjob"),
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
	}

	// Calculate next run time
	nextRun := job.CalculateNextRunTime(now)
	job.NextRunAt = &nextRun

	// Save to database
	err = s.repo.Create(ctx, job)
	if err != nil {
		s.logger.Errorw("failed to create scheduled job", "error", err)
		return nil, ierr.WithError(err).
			WithHint("Failed to create scheduled job").
			Mark(ierr.ErrDatabase)
	}

	s.logger.Infow("scheduled job created successfully",
		"id", job.ID,
		"entity_type", job.EntityType,
		"interval", job.Interval)

	// Start Temporal schedule if enabled
	if job.Enabled && s.orchestrator != nil {
		err = s.orchestrator.StartScheduledJob(ctx, job.ID)
		if err != nil {
			s.logger.Errorw("failed to start temporal schedule", "error", err)
			// Don't fail the creation - job is created, schedule can be started later
		}
	}

	return dto.ToScheduledJobResponse(job), nil
}

// GetScheduledJob retrieves a scheduled job by ID
func (s *scheduledJobService) GetScheduledJob(ctx context.Context, id string) (*dto.ScheduledJobResponse, error) {
	job, err := s.repo.Get(ctx, id)
	if err != nil {
		s.logger.Errorw("failed to get scheduled job", "id", id, "error", err)
		return nil, ierr.WithError(err).
			WithHint("Scheduled job not found").
			Mark(ierr.ErrNotFound)
	}

	return dto.ToScheduledJobResponse(job), nil
}

// ListScheduledJobs retrieves a list of scheduled jobs
func (s *scheduledJobService) ListScheduledJobs(ctx context.Context, filter *types.QueryFilter) (*dto.ListScheduledJobsResponse, error) {
	if filter == nil {
		filter = types.NewDefaultQueryFilter()
	}

	// Convert QueryFilter to ListFilters
	listFilters := &scheduledjob.ListFilters{
		Limit:  int(*filter.Limit),
		Offset: int(*filter.Offset),
	}

	jobs, err := s.repo.List(ctx, listFilters)
	if err != nil {
		s.logger.Errorw("failed to list scheduled jobs", "error", err)
		return nil, ierr.WithError(err).
			WithHint("Failed to retrieve scheduled jobs").
			Mark(ierr.ErrDatabase)
	}

	// Get total count (for pagination)
	totalCount := len(jobs) // TODO: implement proper count query if needed

	return dto.ToScheduledJobListResponse(jobs, totalCount), nil
}

// UpdateScheduledJob updates a scheduled job
func (s *scheduledJobService) UpdateScheduledJob(ctx context.Context, id string, req dto.UpdateScheduledJobRequest) (*dto.ScheduledJobResponse, error) {
	// Get existing job
	job, err := s.repo.Get(ctx, id)
	if err != nil {
		s.logger.Errorw("failed to get scheduled job for update", "id", id, "error", err)
		return nil, ierr.WithError(err).
			WithHint("Scheduled job not found").
			Mark(ierr.ErrNotFound)
	}

	// Track if enabled status changed
	wasEnabled := job.Enabled

	// Update fields if provided
	if req.Interval != nil {
		interval := types.ScheduledJobInterval(*req.Interval)
		if err := interval.Validate(); err != nil {
			return nil, err
		}
		job.Interval = string(interval)
		// Recalculate next run time
		now := time.Now()
		nextRun := job.CalculateNextRunTime(now)
		job.NextRunAt = &nextRun
	}

	if req.Enabled != nil {
		job.Enabled = *req.Enabled
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

		job.JobConfig = *req.JobConfig
	}

	job.UpdatedAt = time.Now()

	// Save updated job
	err = s.repo.Update(ctx, job)
	if err != nil {
		s.logger.Errorw("failed to update scheduled job", "id", id, "error", err)
		return nil, ierr.WithError(err).
			WithHint("Failed to update scheduled job").
			Mark(ierr.ErrDatabase)
	}

	// Handle Temporal schedule enable/disable
	if s.orchestrator != nil && wasEnabled != job.Enabled {
		if job.Enabled {
			// Start/Unpause the schedule
			err = s.orchestrator.StartScheduledJob(ctx, job.ID)
			if err != nil {
				s.logger.Errorw("failed to start temporal schedule", "error", err)
			}
		} else {
			// Pause the schedule
			err = s.orchestrator.StopScheduledJob(ctx, job.ID)
			if err != nil {
				s.logger.Errorw("failed to stop temporal schedule", "error", err)
			}
		}
	}

	s.logger.Infow("scheduled job updated successfully", "id", job.ID)

	return dto.ToScheduledJobResponse(job), nil
}

// DeleteScheduledJob deletes a scheduled job
func (s *scheduledJobService) DeleteScheduledJob(ctx context.Context, id string) error {
	err := s.repo.Delete(ctx, id)
	if err != nil {
		s.logger.Errorw("failed to delete scheduled job", "id", id, "error", err)
		return ierr.WithError(err).
			WithHint("Failed to delete scheduled job").
			Mark(ierr.ErrDatabase)
	}

	s.logger.Infow("scheduled job deleted successfully", "id", id)
	return nil
}

// TriggerManualSync triggers a manual export immediately
func (s *scheduledJobService) TriggerManualSync(ctx context.Context, id string) (string, error) {
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

// GetScheduledJobsByEntityType retrieves scheduled jobs by entity type
func (s *scheduledJobService) GetScheduledJobsByEntityType(ctx context.Context, entityType types.ScheduledJobEntityType) ([]*dto.ScheduledJobResponse, error) {
	jobs, err := s.repo.GetByEntityType(ctx, string(entityType))
	if err != nil {
		s.logger.Errorw("failed to get scheduled jobs by entity type", "entity_type", entityType, "error", err)
		return nil, ierr.WithError(err).
			WithHint("Failed to retrieve scheduled jobs").
			Mark(ierr.ErrDatabase)
	}

	responses := make([]*dto.ScheduledJobResponse, 0, len(jobs))
	for _, job := range jobs {
		responses = append(responses, dto.ToScheduledJobResponse(job))
	}

	return responses, nil
}
