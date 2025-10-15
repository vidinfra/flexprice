package dto

import (
	"time"

	"github.com/flexprice/flexprice/internal/domain/scheduledtask"
	"github.com/flexprice/flexprice/internal/types"
)

// CreateScheduledTaskRequest represents a request to create a scheduled task
type CreateScheduledTaskRequest struct {
	ConnectionID string                 `json:"connection_id" binding:"required"`
	EntityType   string                 `json:"entity_type" binding:"required"`
	Interval     string                 `json:"interval" binding:"required"`
	Enabled      bool                   `json:"enabled"`
	JobConfig    map[string]interface{} `json:"job_config" binding:"required"`
}

// UpdateScheduledTaskRequest represents a request to update a scheduled task
type UpdateScheduledTaskRequest struct {
	Interval  *string                 `json:"interval"`
	Enabled   *bool                   `json:"enabled"`
	JobConfig *map[string]interface{} `json:"job_config"`
}

// ScheduledTaskResponse represents a scheduled task response
type ScheduledTaskResponse struct {
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

// ListScheduledTasksResponse represents a list of scheduled tasks
type ListScheduledTasksResponse struct {
	Data       []ScheduledTaskResponse `json:"data"`
	TotalCount int                     `json:"total_count"`
}

// ToScheduledTaskResponse converts a domain scheduled task to a response
func ToScheduledTaskResponse(task *scheduledtask.ScheduledTask) *ScheduledTaskResponse {
	return &ScheduledTaskResponse{
		ID:            task.ID,
		TenantID:      task.TenantID,
		EnvironmentID: task.EnvironmentID,
		ConnectionID:  task.ConnectionID,
		EntityType:    task.EntityType,
		Interval:      task.Interval,
		Enabled:       task.Enabled,
		JobConfig:     task.JobConfig,
		LastRunAt:     task.LastRunAt,
		NextRunAt:     task.NextRunAt,
		LastRunStatus: task.LastRunStatus,
		LastRunError:  task.LastRunError,
		Status:        task.Status,
		CreatedAt:     task.CreatedAt,
		UpdatedAt:     task.UpdatedAt,
	}
}

// ToScheduledTaskListResponse converts domain scheduled tasks to a list response
func ToScheduledTaskListResponse(tasks []*scheduledtask.ScheduledTask, totalCount int) *ListScheduledTasksResponse {
	responses := make([]ScheduledTaskResponse, 0, len(tasks))
	for _, task := range tasks {
		responses = append(responses, *ToScheduledTaskResponse(task))
	}

	return &ListScheduledTasksResponse{
		Data:       responses,
		TotalCount: totalCount,
	}
}

// ToCreateScheduledTaskInput converts request to domain input
func (r *CreateScheduledTaskRequest) ToCreateInput() *types.CreateScheduledTaskInput {
	enabled := r.Enabled // default will be handled by service if needed
	return &types.CreateScheduledTaskInput{
		ConnectionID: r.ConnectionID,
		EntityType:   types.ScheduledTaskEntityType(r.EntityType),
		Interval:     types.ScheduledTaskInterval(r.Interval),
		Enabled:      enabled,
		JobConfig:    r.JobConfig,
	}
}
