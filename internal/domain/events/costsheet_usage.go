package events

import (
	"context"

	"github.com/flexprice/flexprice/internal/types"
)

// FeatureUsageRepository defines operations for feature usage tracking
type CostTrackingRepository interface {
	// Inserts a single processed event into events_processed table
	InsertProcessedEvent(ctx context.Context, event *FeatureUsage) error

	// Bulk insert events into events_processed table
	BulkInsertProcessedEvents(ctx context.Context, events []*FeatureUsage) error

	// Get processed events with filtering options
	GetProcessedEvents(ctx context.Context, params *GetProcessedEventsParams) ([]*FeatureUsage, uint64, error)
}

// MaxBucketFeatureInfo contains information about a feature that uses MAX with bucket aggregation
type MaxBucketFeatureInfo struct {
	FeatureID    string
	MeterID      string
	BucketSize   types.WindowSize
	EventName    string
	PropertyName string
}
