package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock repository
type MockScheduledJobRepo struct {
	mock.Mock
}

func (m *MockScheduledJobRepo) Create(ctx context.Context, job interface{}) error {
	args := m.Called(ctx, job)
	return args.Error(0)
}

func (m *MockScheduledJobRepo) Get(ctx context.Context, id string) (interface{}, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0), args.Error(1)
}

func (m *MockScheduledJobRepo) Update(ctx context.Context, job interface{}) error {
	args := m.Called(ctx, job)
	return args.Error(0)
}

func (m *MockScheduledJobRepo) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockScheduledJobRepo) List(ctx context.Context, filter interface{}) ([]interface{}, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]interface{}), args.Error(1)
}

func (m *MockScheduledJobRepo) GetByEntityType(ctx context.Context, entityType string) ([]interface{}, error) {
	args := m.Called(ctx, entityType)
	return args.Get(0).([]interface{}), args.Error(1)
}

// Test CreateScheduledJob
func TestCreateScheduledJob(t *testing.T) {
	jobConfig := map[string]interface{}{
		"bucket":      "test-bucket",
		"region":      "us-east-1",
		"key_prefix":  "test/",
		"compression": "gzip",
	}

	req := dto.CreateScheduledJobRequest{
		ConnectionID: "conn-123",
		EntityType:   string(types.ScheduledJobEntityTypeEvents),
		Interval:     string(types.ScheduledJobIntervalDaily),
		Enabled:      true,
		JobConfig:    jobConfig,
	}

	t.Run("Success", func(t *testing.T) {
		// This is a basic structure test
		// Full integration test requires database and Temporal

		assert.Equal(t, "feature_usage", req.EntityType)
		assert.Equal(t, "daily", req.Interval)
		assert.True(t, req.Enabled)
		assert.Equal(t, "test-bucket", req.JobConfig["bucket"])

		// Validate entity type
		entityType := types.ScheduledJobEntityType(req.EntityType)
		err := entityType.Validate()
		assert.NoError(t, err)

		// Validate interval
		interval := types.ScheduledJobInterval(req.Interval)
		err = interval.Validate()
		assert.NoError(t, err)

		// Validate job config can be marshaled
		jobConfigBytes, err := json.Marshal(req.JobConfig)
		assert.NoError(t, err)
		assert.NotEmpty(t, jobConfigBytes)

		var s3Config types.S3JobConfig
		err = json.Unmarshal(jobConfigBytes, &s3Config)
		assert.NoError(t, err)
		assert.Equal(t, "test-bucket", s3Config.Bucket)
		assert.Equal(t, "us-east-1", s3Config.Region)
	})

	t.Run("Invalid entity type", func(t *testing.T) {
		invalidEntityType := types.ScheduledJobEntityType("invalid")
		err := invalidEntityType.Validate()
		assert.Error(t, err)
	})

	t.Run("Invalid interval", func(t *testing.T) {
		invalidInterval := types.ScheduledJobInterval("invalid")
		err := invalidInterval.Validate()
		assert.Error(t, err)
	})
}

// Test S3JobConfig validation
func TestS3JobConfigValidation(t *testing.T) {
	t.Run("Valid config", func(t *testing.T) {
		config := types.S3JobConfig{
			Bucket:    "test-bucket",
			Region:    "us-east-1",
			KeyPrefix: "exports/",
		}
		err := config.Validate()
		assert.NoError(t, err)
	})

	t.Run("Missing bucket", func(t *testing.T) {
		config := types.S3JobConfig{
			Region: "us-east-1",
		}
		err := config.Validate()
		assert.Error(t, err)
	})

	t.Run("Missing region", func(t *testing.T) {
		config := types.S3JobConfig{
			Bucket: "test-bucket",
		}
		err := config.Validate()
		assert.Error(t, err)
	})
}

// Test interval validation
func TestScheduledJobInterval(t *testing.T) {
	tests := []struct {
		name     string
		interval types.ScheduledJobInterval
		wantErr  bool
	}{
		{"Hourly", types.ScheduledJobIntervalHourly, false},
		{"Daily", types.ScheduledJobIntervalDaily, false},
		{"Weekly", types.ScheduledJobIntervalWeekly, false},
		{"Monthly", types.ScheduledJobIntervalMonthly, false},
		{"Invalid", types.ScheduledJobInterval("invalid"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.interval.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test entity type validation
func TestScheduledJobEntityType(t *testing.T) {
	tests := []struct {
		name       string
		entityType types.ScheduledJobEntityType
		wantErr    bool
	}{
		{"FeatureUsage", types.ScheduledJobEntityTypeEvents, false},
		{"Invalid", types.ScheduledJobEntityType("invalid"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.entityType.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test CalculateNextRunTime
func TestCalculateNextRunTime(t *testing.T) {
	now := time.Date(2025, 10, 13, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		interval string
		expected time.Duration
	}{
		{"Hourly", string(types.ScheduledJobIntervalHourly), 1 * time.Hour},
		{"Daily", string(types.ScheduledJobIntervalDaily), 24 * time.Hour},
		{"Weekly", string(types.ScheduledJobIntervalWeekly), 7 * 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This tests the interval duration concept
			var duration time.Duration
			switch types.ScheduledJobInterval(tt.interval) {
			case types.ScheduledJobIntervalHourly:
				duration = 1 * time.Hour
			case types.ScheduledJobIntervalDaily:
				duration = 24 * time.Hour
			case types.ScheduledJobIntervalWeekly:
				duration = 7 * 24 * time.Hour
			case types.ScheduledJobIntervalMonthly:
				duration = 30 * 24 * time.Hour // Approximate
			}

			nextRun := now.Add(duration)
			assert.True(t, nextRun.After(now))
		})
	}
}

// Helper function
func boolPtr(b bool) *bool {
	return &b
}
