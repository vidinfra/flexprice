package scheduledtask

import (
	"time"

	"github.com/flexprice/flexprice/internal/types"
)

// ScheduledTask represents a scheduled export task
type ScheduledTask struct {
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

// IsEnabled returns whether the task is enabled
func (j *ScheduledTask) IsEnabled() bool {
	return j.Enabled && j.Status == "published"
}

// IsDue checks if the task is due for execution
func (j *ScheduledTask) IsDue(currentTime time.Time) bool {
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
// This should match the Temporal cron schedule (boundary-aligned)
func (j *ScheduledTask) CalculateNextRunTime(fromTime time.Time) time.Time {
	switch types.ScheduledTaskInterval(j.Interval) {
	case types.ScheduledTaskIntervalTesting:
		// Align to next 10-minute boundary
		// Example: 8:47 → 8:50, 8:50 → 9:00
		minutes := (fromTime.Minute()/10 + 1) * 10
		if minutes >= 60 {
			return time.Date(fromTime.Year(), fromTime.Month(), fromTime.Day(),
				fromTime.Hour()+1, 0, 0, 0, fromTime.Location())
		}
		return time.Date(fromTime.Year(), fromTime.Month(), fromTime.Day(),
			fromTime.Hour(), minutes, 0, 0, fromTime.Location())

	case types.ScheduledTaskIntervalHourly:
		// Align to next hour at 15 minutes past (cron: 15 * * * *)
		// Example: 10:05 → 10:15, 10:20 → 11:15
		if fromTime.Minute() < 15 {
			// Next run is at :15 of current hour
			return time.Date(fromTime.Year(), fromTime.Month(), fromTime.Day(),
				fromTime.Hour(), 15, 0, 0, fromTime.Location())
		}
		// Next run is at :15 of next hour
		return time.Date(fromTime.Year(), fromTime.Month(), fromTime.Day(),
			fromTime.Hour()+1, 15, 0, 0, fromTime.Location())

	case types.ScheduledTaskIntervalDaily:
		// Align to next day at 00:15 (cron: 15 0 * * *)
		nextDay := time.Date(fromTime.Year(), fromTime.Month(), fromTime.Day(),
			0, 15, 0, 0, fromTime.Location())
		if fromTime.Before(nextDay) {
			return nextDay
		}
		return nextDay.AddDate(0, 0, 1)

	case types.ScheduledTaskIntervalWeekly:
		// Align to next Monday at 00:15 (cron: 15 0 * * 1)
		weekday := fromTime.Weekday()
		var daysUntilMonday int
		if weekday == time.Sunday {
			daysUntilMonday = 1
		} else {
			daysUntilMonday = 8 - int(weekday)
		}
		nextMonday := time.Date(fromTime.Year(), fromTime.Month(), fromTime.Day(),
			0, 15, 0, 0, fromTime.Location()).AddDate(0, 0, daysUntilMonday)
		if fromTime.Before(nextMonday) && weekday == time.Monday {
			return nextMonday
		}
		return time.Date(fromTime.Year(), fromTime.Month(), fromTime.Day(),
			0, 15, 0, 0, fromTime.Location()).AddDate(0, 0, daysUntilMonday)

	case types.ScheduledTaskIntervalMonthly:
		// Align to first day of next month at 00:15 (cron: 15 0 1 * *)
		nextMonth := time.Date(fromTime.Year(), fromTime.Month(), 1,
			0, 15, 0, 0, fromTime.Location())
		if fromTime.Before(nextMonth) {
			return nextMonth
		}
		return nextMonth.AddDate(0, 1, 0)

	case types.ScheduledTaskIntervalYearly:
		// Align to Jan 1st of next year at 00:15 (cron: 15 0 1 1 *)
		nextYear := time.Date(fromTime.Year(), time.January, 1,
			0, 15, 0, 0, fromTime.Location())
		if fromTime.Before(nextYear) {
			return nextYear
		}
		return time.Date(fromTime.Year()+1, time.January, 1,
			0, 0, 0, 0, fromTime.Location())

	default:
		// Default to next midnight
		return time.Date(fromTime.Year(), fromTime.Month(), fromTime.Day(),
			0, 0, 0, 0, fromTime.Location()).AddDate(0, 0, 1)
	}
}

// GetS3JobConfig parses the task config as S3JobConfig
func (j *ScheduledTask) GetS3JobConfig() (*types.S3JobConfig, error) {
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
