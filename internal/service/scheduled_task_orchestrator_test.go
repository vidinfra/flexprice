package service

import (
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateIntervalBoundaries(t *testing.T) {
	// Create orchestrator with minimal dependencies for testing
	cfg := &config.Configuration{}
	log, err := logger.NewLogger(cfg)
	require.NoError(t, err)

	orchestrator := &ScheduledTaskOrchestrator{
		logger: log,
	}

	// Use a fixed timezone for consistent testing
	loc := time.UTC

	tests := []struct {
		name              string
		currentTime       time.Time
		interval          types.ScheduledTaskInterval
		expectedStartTime time.Time
		expectedEndTime   time.Time
	}{
		{
			name:              "Hourly - 10:30 AM should give 10:00-11:00",
			currentTime:       time.Date(2025, 10, 16, 10, 30, 0, 0, loc),
			interval:          types.ScheduledTaskIntervalHourly,
			expectedStartTime: time.Date(2025, 10, 16, 10, 0, 0, 0, loc),
			expectedEndTime:   time.Date(2025, 10, 16, 11, 0, 0, 0, loc),
		},
		{
			name:              "Hourly - exact hour boundary",
			currentTime:       time.Date(2025, 10, 16, 10, 0, 0, 0, loc),
			interval:          types.ScheduledTaskIntervalHourly,
			expectedStartTime: time.Date(2025, 10, 16, 10, 0, 0, 0, loc),
			expectedEndTime:   time.Date(2025, 10, 16, 11, 0, 0, 0, loc),
		},
		{
			name:              "Hourly - 10:59:59 should give 10:00-11:00",
			currentTime:       time.Date(2025, 10, 16, 10, 59, 59, 0, loc),
			interval:          types.ScheduledTaskIntervalHourly,
			expectedStartTime: time.Date(2025, 10, 16, 10, 0, 0, 0, loc),
			expectedEndTime:   time.Date(2025, 10, 16, 11, 0, 0, 0, loc),
		},
		{
			name:              "Daily - 6:30 PM on Oct 16 should give Oct 16 00:00 - Oct 17 00:00",
			currentTime:       time.Date(2025, 10, 16, 18, 30, 0, 0, loc),
			interval:          types.ScheduledTaskIntervalDaily,
			expectedStartTime: time.Date(2025, 10, 16, 0, 0, 0, 0, loc),
			expectedEndTime:   time.Date(2025, 10, 17, 0, 0, 0, 0, loc),
		},
		{
			name:              "Daily - midnight should align to same day",
			currentTime:       time.Date(2025, 10, 16, 0, 0, 0, 0, loc),
			interval:          types.ScheduledTaskIntervalDaily,
			expectedStartTime: time.Date(2025, 10, 16, 0, 0, 0, 0, loc),
			expectedEndTime:   time.Date(2025, 10, 17, 0, 0, 0, 0, loc),
		},
		{
			name:              "Weekly - Thursday Oct 16 should give Monday Oct 13 - Monday Oct 20",
			currentTime:       time.Date(2025, 10, 16, 10, 30, 0, 0, loc), // Thursday
			interval:          types.ScheduledTaskIntervalWeekly,
			expectedStartTime: time.Date(2025, 10, 13, 0, 0, 0, 0, loc), // Monday
			expectedEndTime:   time.Date(2025, 10, 20, 0, 0, 0, 0, loc), // Next Monday
		},
		{
			name:              "Weekly - Monday should give same Monday - next Monday",
			currentTime:       time.Date(2025, 10, 13, 10, 30, 0, 0, loc), // Monday
			interval:          types.ScheduledTaskIntervalWeekly,
			expectedStartTime: time.Date(2025, 10, 13, 0, 0, 0, 0, loc), // Same Monday
			expectedEndTime:   time.Date(2025, 10, 20, 0, 0, 0, 0, loc), // Next Monday
		},
		{
			name:              "Weekly - Sunday should give previous Monday - next Monday",
			currentTime:       time.Date(2025, 10, 19, 10, 30, 0, 0, loc), // Sunday
			interval:          types.ScheduledTaskIntervalWeekly,
			expectedStartTime: time.Date(2025, 10, 13, 0, 0, 0, 0, loc), // Previous Monday
			expectedEndTime:   time.Date(2025, 10, 20, 0, 0, 0, 0, loc), // Next Monday
		},
		{
			name:              "Monthly - Oct 16 should give Oct 1 - Nov 1",
			currentTime:       time.Date(2025, 10, 16, 10, 30, 0, 0, loc),
			interval:          types.ScheduledTaskIntervalMonthly,
			expectedStartTime: time.Date(2025, 10, 1, 0, 0, 0, 0, loc),
			expectedEndTime:   time.Date(2025, 11, 1, 0, 0, 0, 0, loc),
		},
		{
			name:              "Monthly - first day of month",
			currentTime:       time.Date(2025, 10, 1, 10, 30, 0, 0, loc),
			interval:          types.ScheduledTaskIntervalMonthly,
			expectedStartTime: time.Date(2025, 10, 1, 0, 0, 0, 0, loc),
			expectedEndTime:   time.Date(2025, 11, 1, 0, 0, 0, 0, loc),
		},
		{
			name:              "Monthly - last day of month",
			currentTime:       time.Date(2025, 10, 31, 23, 59, 59, 0, loc),
			interval:          types.ScheduledTaskIntervalMonthly,
			expectedStartTime: time.Date(2025, 10, 1, 0, 0, 0, 0, loc),
			expectedEndTime:   time.Date(2025, 11, 1, 0, 0, 0, 0, loc),
		},
		{
			name:              "Yearly - Oct 16 2025 should give Jan 1 2025 - Jan 1 2026",
			currentTime:       time.Date(2025, 10, 16, 10, 30, 0, 0, loc),
			interval:          types.ScheduledTaskIntervalYearly,
			expectedStartTime: time.Date(2025, 1, 1, 0, 0, 0, 0, loc),
			expectedEndTime:   time.Date(2026, 1, 1, 0, 0, 0, 0, loc),
		},
		{
			name:              "Yearly - first day of year",
			currentTime:       time.Date(2025, 1, 1, 10, 30, 0, 0, loc),
			interval:          types.ScheduledTaskIntervalYearly,
			expectedStartTime: time.Date(2025, 1, 1, 0, 0, 0, 0, loc),
			expectedEndTime:   time.Date(2026, 1, 1, 0, 0, 0, 0, loc),
		},
		{
			name:              "Yearly - last day of year",
			currentTime:       time.Date(2025, 12, 31, 23, 59, 59, 0, loc),
			interval:          types.ScheduledTaskIntervalYearly,
			expectedStartTime: time.Date(2025, 1, 1, 0, 0, 0, 0, loc),
			expectedEndTime:   time.Date(2026, 1, 1, 0, 0, 0, 0, loc),
		},
		{
			name:              "Testing - 10:35 should align to 10:30-10:40",
			currentTime:       time.Date(2025, 10, 16, 10, 35, 0, 0, loc),
			interval:          types.ScheduledTaskIntervalTesting,
			expectedStartTime: time.Date(2025, 10, 16, 10, 30, 0, 0, loc),
			expectedEndTime:   time.Date(2025, 10, 16, 10, 40, 0, 0, loc),
		},
		{
			name:              "Testing - 10:42 should align to 10:40-10:50",
			currentTime:       time.Date(2025, 10, 16, 10, 42, 0, 0, loc),
			interval:          types.ScheduledTaskIntervalTesting,
			expectedStartTime: time.Date(2025, 10, 16, 10, 40, 0, 0, loc),
			expectedEndTime:   time.Date(2025, 10, 16, 10, 50, 0, 0, loc),
		},
		{
			name:              "Testing - exact 10-minute boundary",
			currentTime:       time.Date(2025, 10, 16, 10, 30, 0, 0, loc),
			interval:          types.ScheduledTaskIntervalTesting,
			expectedStartTime: time.Date(2025, 10, 16, 10, 30, 0, 0, loc),
			expectedEndTime:   time.Date(2025, 10, 16, 10, 40, 0, 0, loc),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startTime, endTime := orchestrator.CalculateIntervalBoundaries(tt.currentTime, tt.interval)

			assert.Equal(t, tt.expectedStartTime, startTime,
				"Start time mismatch for %s. Expected %s, got %s",
				tt.name, tt.expectedStartTime.Format(time.RFC3339), startTime.Format(time.RFC3339))

			assert.Equal(t, tt.expectedEndTime, endTime,
				"End time mismatch for %s. Expected %s, got %s",
				tt.name, tt.expectedEndTime.Format(time.RFC3339), endTime.Format(time.RFC3339))

			// Verify that start time is before end time
			assert.True(t, startTime.Before(endTime),
				"Start time should be before end time for %s", tt.name)

			// Verify that current time falls within or at the start boundary
			assert.True(t, !tt.currentTime.Before(startTime),
				"Current time should not be before start time for %s", tt.name)
		})
	}
}

func TestCalculateIntervalBoundaries_EdgeCases(t *testing.T) {
	cfg := &config.Configuration{}
	log, err := logger.NewLogger(cfg)
	require.NoError(t, err)

	orchestrator := &ScheduledTaskOrchestrator{
		logger: log,
	}

	loc := time.UTC

	t.Run("Weekly - handles year boundary correctly", func(t *testing.T) {
		// Dec 31, 2025 is a Wednesday
		currentTime := time.Date(2025, 12, 31, 10, 30, 0, 0, loc)
		startTime, endTime := orchestrator.CalculateIntervalBoundaries(currentTime, types.ScheduledTaskIntervalWeekly)

		// Should align to Monday Dec 29, 2025 - Monday Jan 5, 2026
		expectedStart := time.Date(2025, 12, 29, 0, 0, 0, 0, loc)
		expectedEnd := time.Date(2026, 1, 5, 0, 0, 0, 0, loc)

		assert.Equal(t, expectedStart, startTime)
		assert.Equal(t, expectedEnd, endTime)
	})

	t.Run("Monthly - handles leap year February", func(t *testing.T) {
		// 2024 is a leap year
		currentTime := time.Date(2024, 2, 29, 10, 30, 0, 0, loc)
		startTime, endTime := orchestrator.CalculateIntervalBoundaries(currentTime, types.ScheduledTaskIntervalMonthly)

		expectedStart := time.Date(2024, 2, 1, 0, 0, 0, 0, loc)
		expectedEnd := time.Date(2024, 3, 1, 0, 0, 0, 0, loc)

		assert.Equal(t, expectedStart, startTime)
		assert.Equal(t, expectedEnd, endTime)
	})

	t.Run("Monthly - handles December to January transition", func(t *testing.T) {
		currentTime := time.Date(2025, 12, 15, 10, 30, 0, 0, loc)
		startTime, endTime := orchestrator.CalculateIntervalBoundaries(currentTime, types.ScheduledTaskIntervalMonthly)

		expectedStart := time.Date(2025, 12, 1, 0, 0, 0, 0, loc)
		expectedEnd := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)

		assert.Equal(t, expectedStart, startTime)
		assert.Equal(t, expectedEnd, endTime)
	})

	t.Run("Hourly - handles day boundary", func(t *testing.T) {
		currentTime := time.Date(2025, 10, 16, 23, 30, 0, 0, loc)
		startTime, endTime := orchestrator.CalculateIntervalBoundaries(currentTime, types.ScheduledTaskIntervalHourly)

		expectedStart := time.Date(2025, 10, 16, 23, 0, 0, 0, loc)
		expectedEnd := time.Date(2025, 10, 17, 0, 0, 0, 0, loc)

		assert.Equal(t, expectedStart, startTime)
		assert.Equal(t, expectedEnd, endTime)
	})
}

func TestGetCronExpression(t *testing.T) {
	cfg := &config.Configuration{}
	log, err := logger.NewLogger(cfg)
	require.NoError(t, err)

	orchestrator := &ScheduledTaskOrchestrator{
		logger: log,
	}

	tests := []struct {
		name         string
		interval     types.ScheduledTaskInterval
		expectedCron string
	}{
		{
			name:         "Testing interval",
			interval:     types.ScheduledTaskIntervalTesting,
			expectedCron: "*/10 * * * *",
		},
		{
			name:         "Hourly interval",
			interval:     types.ScheduledTaskIntervalHourly,
			expectedCron: "0 * * * *",
		},
		{
			name:         "Daily interval",
			interval:     types.ScheduledTaskIntervalDaily,
			expectedCron: "0 0 * * *",
		},
		{
			name:         "Weekly interval",
			interval:     types.ScheduledTaskIntervalWeekly,
			expectedCron: "0 0 * * 1", // Monday at midnight
		},
		{
			name:         "Monthly interval",
			interval:     types.ScheduledTaskIntervalMonthly,
			expectedCron: "0 0 1 * *",
		},
		{
			name:         "Yearly interval",
			interval:     types.ScheduledTaskIntervalYearly,
			expectedCron: "0 0 1 1 *",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cron := orchestrator.getCronExpression(tt.interval)
			assert.Equal(t, tt.expectedCron, cron)
		})
	}
}
