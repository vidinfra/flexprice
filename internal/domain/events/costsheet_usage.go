package events

import (
	"context"
	"time"
)

// FeatureUsageRepository defines operations for feature usage tracking
type CostSheetUsageRepository interface {
	// Inserts a single processed event into events_processed table
	InsertProcessedEvent(ctx context.Context, event *CostUsage) error

	// Bulk insert events into events_processed table
	BulkInsertProcessedEvents(ctx context.Context, events []*CostUsage) error

	// Get processed events with filtering options
	GetProcessedEvents(ctx context.Context, params *GetCostUsageEventsParams) ([]*CostUsage, uint64, error)

	// Get usage by cost sheet ID
	GetUsageByCostSheetID(ctx context.Context, costSheetID, externalCustomerID string, startTime, endTime time.Time) (map[string]*UsageByCostSheetResult, error)
}
