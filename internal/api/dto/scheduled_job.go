package dto

import (
	"time"

	"github.com/flexprice/flexprice/internal/domain/scheduledjob"
	"github.com/flexprice/flexprice/internal/types"
)

// CreateScheduledJobRequest represents a request to create a scheduled job
type CreateScheduledJobRequest struct {
	ConnectionID string                 `json:"connection_id" binding:"required"`
	EntityType   string                 `json:"entity_type" binding:"required"`
	Interval     string                 `json:"interval" binding:"required"`
	Enabled      bool                   `json:"enabled"`
	JobConfig    map[string]interface{} `json:"job_config" binding:"required"`
}

// UpdateScheduledJobRequest represents a request to update a scheduled job
type UpdateScheduledJobRequest struct {
	Interval  *string                 `json:"interval"`
	Enabled   *bool                   `json:"enabled"`
	JobConfig *map[string]interface{} `json:"job_config"`
}

// ScheduledJobResponse represents a scheduled job response
type ScheduledJobResponse struct {
	ID            string                 `json:"id"`
	TenantID      string                 `json:"tenant_id"`
	EnvironmentID string                 `json:"environment_id"`
	ConnectionID  string                 `json:"connection_id"`
	EntityType    string                 `json:"entity_type"`
	Interval      string                 `json:"interval"`
	Enabled       bool                   `json:"enabled"`
	JobConfig     map[string]interface{} `json:"job_config"`
	LastRunAt     *time.Time             `json:"last_run_at,omitempty"`
	NextRunAt     *time.Time             `json:"next_run_at,omitempty"`
	LastRunStatus string                 `json:"last_run_status,omitempty"`
	LastRunError  string                 `json:"last_run_error,omitempty"`
	Status        string                 `json:"status"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

// ListScheduledJobsResponse represents a list of scheduled jobs
type ListScheduledJobsResponse struct {
	Data       []ScheduledJobResponse `json:"data"`
	TotalCount int                    `json:"total_count"`
}

// ToScheduledJobResponse converts a domain scheduled job to a response
func ToScheduledJobResponse(job *scheduledjob.ScheduledJob) *ScheduledJobResponse {
	return &ScheduledJobResponse{
		ID:            job.ID,
		TenantID:      job.TenantID,
		EnvironmentID: job.EnvironmentID,
		ConnectionID:  job.ConnectionID,
		EntityType:    job.EntityType,
		Interval:      job.Interval,
		Enabled:       job.Enabled,
		JobConfig:     job.JobConfig,
		LastRunAt:     job.LastRunAt,
		NextRunAt:     job.NextRunAt,
		LastRunStatus: job.LastRunStatus,
		LastRunError:  job.LastRunError,
		Status:        job.Status,
		CreatedAt:     job.CreatedAt,
		UpdatedAt:     job.UpdatedAt,
	}
}

// ToScheduledJobListResponse converts domain scheduled jobs to a list response
func ToScheduledJobListResponse(jobs []*scheduledjob.ScheduledJob, totalCount int) *ListScheduledJobsResponse {
	responses := make([]ScheduledJobResponse, 0, len(jobs))
	for _, job := range jobs {
		responses = append(responses, *ToScheduledJobResponse(job))
	}

	return &ListScheduledJobsResponse{
		Data:       responses,
		TotalCount: totalCount,
	}
}

// ToCreateScheduledJobInput converts request to domain input
func (r *CreateScheduledJobRequest) ToCreateInput() *types.CreateScheduledJobInput {
	enabled := r.Enabled // default will be handled by service if needed
	return &types.CreateScheduledJobInput{
		ConnectionID: r.ConnectionID,
		EntityType:   types.ScheduledJobEntityType(r.EntityType),
		Interval:     types.ScheduledJobInterval(r.Interval),
		Enabled:      enabled,
		JobConfig:    r.JobConfig,
	}
}
