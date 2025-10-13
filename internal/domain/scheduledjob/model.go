package scheduledjob

import (
	"time"

	"github.com/flexprice/flexprice/internal/types"
)

// ScheduledJob represents a scheduled export job
type ScheduledJob struct {
	ID                 string
	TenantID           string
	EnvironmentID      string
	ConnectionID       string
	EntityType         string
	Interval           string
	Enabled            bool
	JobConfig          map[string]interface{}
	LastRunAt          *time.Time
	NextRunAt          *time.Time
	LastRunStatus      string
	LastRunError       string
	TemporalScheduleID string // Temporal schedule ID
	Status             string // "published" or "draft"
	CreatedAt          time.Time
	UpdatedAt          time.Time
	CreatedBy          string
	UpdatedBy          string
}

// IsEnabled returns whether the job is enabled
func (j *ScheduledJob) IsEnabled() bool {
	return j.Enabled && j.Status == "published"
}

// IsDue checks if the job is due for execution
func (j *ScheduledJob) IsDue(currentTime time.Time) bool {
	if !j.IsEnabled() {
		return false
	}

	// If never run, it's due
	if j.NextRunAt == nil {
		return true
	}

	// Check if current time is after or equal to next run time
	return currentTime.After(*j.NextRunAt) || currentTime.Equal(*j.NextRunAt)
}

// CalculateNextRunTime calculates the next run time based on the interval
func (j *ScheduledJob) CalculateNextRunTime(fromTime time.Time) time.Time {
	switch types.ScheduledJobInterval(j.Interval) {
	case types.ScheduledJobIntervalHourly:
		return fromTime.Add(1 * time.Hour)
	case types.ScheduledJobIntervalDaily:
		return fromTime.Add(24 * time.Hour)
	case types.ScheduledJobIntervalWeekly:
		return fromTime.Add(7 * 24 * time.Hour)
	case types.ScheduledJobIntervalMonthly:
		return fromTime.AddDate(0, 1, 0)
	default:
		// Default to daily
		return fromTime.Add(24 * time.Hour)
	}
}

// GetS3JobConfig parses the job config as S3JobConfig
func (j *ScheduledJob) GetS3JobConfig() (*types.S3JobConfig, error) {
	if j.JobConfig == nil {
		return nil, nil
	}

	config := &types.S3JobConfig{}

	// Parse from map
	if bucket, ok := j.JobConfig["bucket"].(string); ok {
		config.Bucket = bucket
	}
	if region, ok := j.JobConfig["region"].(string); ok {
		config.Region = region
	}
	if keyPrefix, ok := j.JobConfig["key_prefix"].(string); ok {
		config.KeyPrefix = keyPrefix
	}
	if compression, ok := j.JobConfig["compression"].(string); ok {
		config.Compression = compression
	}
	if encryption, ok := j.JobConfig["encryption"].(string); ok {
		config.Encryption = encryption
	}
	if endpointURL, ok := j.JobConfig["endpoint_url"].(string); ok {
		config.EndpointURL = endpointURL
	}
	if virtualHostStyle, ok := j.JobConfig["virtual_host_style"].(bool); ok {
		config.VirtualHostStyle = virtualHostStyle
	}
	if maxFileSizeMB, ok := j.JobConfig["max_file_size_mb"].(float64); ok {
		config.MaxFileSizeMB = int(maxFileSizeMB)
	}

	// Set defaults
	config.SetDefaults()

	return config, config.Validate()
}
