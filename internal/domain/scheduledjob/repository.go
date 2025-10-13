package scheduledjob

import (
	"context"
	"time"
)

// Repository defines the interface for scheduled job persistence operations
type Repository interface {
	// Create creates a new scheduled job
	Create(ctx context.Context, job *ScheduledJob) error

	// Get retrieves a scheduled job by ID
	Get(ctx context.Context, id string) (*ScheduledJob, error)

	// Update updates an existing scheduled job
	Update(ctx context.Context, job *ScheduledJob) error

	// Delete deletes a scheduled job
	Delete(ctx context.Context, id string) error

	// List retrieves all scheduled jobs with optional filters
	List(ctx context.Context, filters *ListFilters) ([]*ScheduledJob, error)

	// GetByConnection retrieves all scheduled jobs for a specific connection
	GetByConnection(ctx context.Context, connectionID string) ([]*ScheduledJob, error)

	// GetByEntityType retrieves all enabled scheduled jobs for a specific entity type
	GetByEntityType(ctx context.Context, entityType string) ([]*ScheduledJob, error)

	// GetJobsDueForExecution retrieves all enabled jobs that are due for execution
	GetJobsDueForExecution(ctx context.Context, currentTime time.Time) ([]*ScheduledJob, error)

	// UpdateLastRun updates the last run information for a scheduled job
	UpdateLastRun(ctx context.Context, jobID string, runTime time.Time, nextRunTime time.Time, status string, errorMsg string) error
}

// ListFilters defines filters for listing scheduled jobs
type ListFilters struct {
	TenantID      string
	EnvironmentID string
	ConnectionID  string
	EntityType    string
	Interval      string
	Enabled       *bool
	Limit         int
	Offset        int
}
