package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/message/router/middleware"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/domain/meter"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/pubsub"
	"github.com/flexprice/flexprice/internal/pubsub/kafka"
	pubsubRouter "github.com/flexprice/flexprice/internal/pubsub/router"
	"github.com/flexprice/flexprice/internal/types"
)

// CostTrackingService handles cost tracking operations for metered events
type CostSheetTrackingService interface {
	// Publish an event for feature usage tracking
	PublishEvent(ctx context.Context, event *events.Event, isBackfill bool) error

	// Register message handler with the router
	RegisterHandler(router *pubsubRouter.Router, cfg *config.Configuration)
}

type costsheetTrackingService struct {
	ServiceParams
	pubSub    pubsub.PubSub
	eventRepo events.Repository
}

// NewCostTrackingService creates a new cost tracking service
func NewCostTrackingService(
	params ServiceParams,
	eventRepo events.Repository,
) CostSheetTrackingService {
	ev := &costsheetTrackingService{
		ServiceParams: params,
		eventRepo:     eventRepo,
	}

	pubSub, err := kafka.NewPubSubFromConfig(
		params.Config,
		params.Logger,
		params.Config.CostTracking.ConsumerGroup,
	)

	if err != nil {
		params.Logger.Fatalw("failed to create pubsub", "error", err)
		return nil
	}
	ev.pubSub = pubSub

	return ev
}

// PublishEvent publishes an event to the feature usage tracking topic
func (s *costsheetTrackingService) PublishEvent(ctx context.Context, event *events.Event, isBackfill bool) error {
	// Create message payload
	payload, err := json.Marshal(event)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to marshal event for feature usage tracking").
			Mark(ierr.ErrValidation)
	}

	// Create a deterministic partition key based on tenant_id and external_customer_id
	// This ensures all events for the same customer go to the same partition
	partitionKey := event.TenantID
	if event.ExternalCustomerID != "" {
		partitionKey = fmt.Sprintf("%s:%s", event.TenantID, event.ExternalCustomerID)
	}

	// Make UUID truly unique by adding nanosecond precision timestamp and random bytes
	uniqueID := fmt.Sprintf("%s-%d-%d", event.ID, time.Now().UnixNano(), rand.Int63())

	// Use the partition key as the message ID to ensure consistent partitioning
	msg := message.NewMessage(uniqueID, payload)

	// Set metadata for additional context
	msg.Metadata.Set("tenant_id", event.TenantID)
	msg.Metadata.Set("environment_id", event.EnvironmentID)
	msg.Metadata.Set("partition_key", partitionKey)

	pubSub := s.pubSub
	topic := s.Config.CostTracking.Topic

	if pubSub == nil {
		return ierr.NewError("pubsub not initialized").
			WithHint("Please check the config").
			Mark(ierr.ErrSystem)
	}

	s.Logger.Debugw("publishing event for cost tracking",
		"event_id", event.ID,
		"event_name", event.EventName,
		"partition_key", partitionKey,
		"topic", topic,
	)

	// Publish to feature usage tracking topic using the backfill PubSub (Kafka)
	if err := pubSub.Publish(ctx, topic, msg); err != nil {
		return ierr.WithError(err).
			WithHint("Failed to publish event for feature usage tracking").
			Mark(ierr.ErrSystem)
	}
	return nil
}

// RegisterHandler registers a handler for the feature usage tracking topic with rate limiting
func (s *costsheetTrackingService) RegisterHandler(router *pubsubRouter.Router, cfg *config.Configuration) {
	// Add throttle middleware to this specific handler
	throttle := middleware.NewThrottle(cfg.FeatureUsageTracking.RateLimit, time.Second)

	// Add the handler
	router.AddNoPublishHandler(
		"cost_tracking_handler",
		cfg.CostTracking.Topic,
		s.pubSub,
		s.processMessage,
		throttle.Middleware,
	)

	s.Logger.Infow("registered event cost tracking handler",
		"topic", cfg.CostTracking.Topic,
		"rate_limit", cfg.CostTracking.RateLimit,
	)

}

// Process a single event message for feature usage tracking
func (s *costsheetTrackingService) processMessage(msg *message.Message) error {
	// Extract tenant ID from message metadata
	partitionKey := msg.Metadata.Get("partition_key")
	tenantID := msg.Metadata.Get("tenant_id")
	environmentID := msg.Metadata.Get("environment_id")

	s.Logger.Debugw("processing event from message queue",
		"message_uuid", msg.UUID,
		"partition_key", partitionKey,
		"tenant_id", tenantID,
		"environment_id", environmentID,
	)

	// Create a background context with tenant ID
	ctx := context.Background()
	if tenantID != "" {
		ctx = context.WithValue(ctx, types.CtxTenantID, tenantID)
	}

	if environmentID != "" {
		ctx = context.WithValue(ctx, types.CtxEnvironmentID, environmentID)
	}

	// Unmarshal the event
	var event events.Event
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		s.Logger.Errorw("failed to unmarshal event for feature usage tracking",
			"error", err,
			"message_uuid", msg.UUID,
		)
		return nil // Don't retry on unmarshal errors
	}

	// validate tenant id
	if event.TenantID != tenantID {
		s.Logger.Errorw("invalid tenant id",
			"expected", tenantID,
			"actual", event.TenantID,
			"message_uuid", msg.UUID,
		)
		return nil // Don't retry on invalid tenant id
	}

	// Process the event
	if err := s.processEvent(ctx, &event); err != nil {
		s.Logger.Errorw("failed to process event for feature usage tracking",
			"error", err,
			"event_id", event.ID,
			"event_name", event.EventName,
		)
		return err // Return error for retry
	}

	s.Logger.Infow("event for feature usage tracking processed successfully",
		"event_id", event.ID,
		"event_name", event.EventName,
	)

	return nil
}

// Process a single event for feature usage tracking
func (s *costsheetTrackingService) processEvent(ctx context.Context, event *events.Event) error {
	s.Logger.Debugw("processing event",
		"event_id", event.ID,
		"event_name", event.EventName,
		"external_customer_id", event.ExternalCustomerID,
		"ingested_at", event.IngestedAt,
	)

	costUsage, err := s.prepareProcessedEvents(ctx, event)
	if err != nil {
		s.Logger.Errorw("failed to prepare cost usage",
			"error", err,
			"event_id", event.ID,
		)
		return err
	}

	if len(costUsage) > 0 {
		// if err := s.costUsageRepo.BulkInsertProcessedEvents(ctx, costUsage); err != nil {
		// 	return err
		// }
		return nil
	}

	return nil
}

// Generate a unique hash for deduplication
// there are 2 cases:
// 1. event_name + event_id // for non COUNT_UNIQUE aggregation types
// 2. event_name + event_field_name + event_field_value // for COUNT_UNIQUE aggregation types
func (s *costsheetTrackingService) generateUniqueHash(event *events.Event, meter *meter.Meter) string {
	hashStr := fmt.Sprintf("%s:%s", event.EventName, event.ID)

	// For meters with field-based aggregation, include the field value in the hash
	if meter.Aggregation.Type == types.AggregationCountUnique && meter.Aggregation.Field != "" {
		if fieldValue, ok := event.Properties[meter.Aggregation.Field]; ok {
			hashStr = fmt.Sprintf("%s:%s:%v", event.EventName, meter.Aggregation.Field, fieldValue)
		}
	}

	hash := sha256.Sum256([]byte(hashStr))
	return hex.EncodeToString(hash[:])
}

func (s *costsheetTrackingService) prepareProcessedEvents(ctx context.Context, event *events.Event) ([]*events.CostUsage, error) {
	return nil, nil
}

// ReprocessEvents triggers reprocessing of events for a customer or with other filters
func (s *costsheetTrackingService) ReprocessEvents(ctx context.Context, params *events.ReprocessEventsParams) error {
	s.Logger.Infow("starting event reprocessing for feature usage tracking",
		"external_customer_id", params.ExternalCustomerID,
		"event_name", params.EventName,
		"start_time", params.StartTime,
		"end_time", params.EndTime,
	)

	// Set default batch size if not provided
	batchSize := params.BatchSize
	if batchSize <= 0 {
		batchSize = 100
	}

	// Create find params from reprocess params
	findParams := &events.FindUnprocessedEventsParams{
		ExternalCustomerID: params.ExternalCustomerID,
		EventName:          params.EventName,
		StartTime:          params.StartTime,
		EndTime:            params.EndTime,
		BatchSize:          batchSize,
	}

	// We'll process in batches to avoid memory issues with large datasets
	processedBatches := 0
	totalEventsFound := 0
	totalEventsPublished := 0
	var lastID string
	var lastTimestamp time.Time

	// Keep processing batches until we're done
	for {
		// Update keyset pagination parameters for next batch
		if lastID != "" && !lastTimestamp.IsZero() {
			findParams.LastID = lastID
			findParams.LastTimestamp = lastTimestamp
		}

		// Find unprocessed events
		unprocessedEvents, err := s.eventRepo.FindUnprocessedEventsFromFeatureUsage(ctx, findParams)
		if err != nil {
			return ierr.WithError(err).
				WithHint("Failed to find unprocessed events").
				WithReportableDetails(map[string]interface{}{
					"external_customer_id": params.ExternalCustomerID,
					"event_name":           params.EventName,
					"batch":                processedBatches,
				}).
				Mark(ierr.ErrDatabase)
		}

		eventsCount := len(unprocessedEvents)
		totalEventsFound += eventsCount
		s.Logger.Infow("found unprocessed events",
			"batch", processedBatches,
			"count", eventsCount,
			"total_found", totalEventsFound,
		)

		// If no more events, we're done
		if eventsCount == 0 {
			break
		}

		// Publish each event to the feature usage tracking topic
		for _, event := range unprocessedEvents {
			// hardcoded delay to avoid rate limiting
			// TODO: remove this to make it configurable
			if err := s.PublishEvent(ctx, event, true); err != nil {
				s.Logger.Errorw("failed to publish event for reprocessing for feature usage tracking",
					"event_id", event.ID,
					"error", err,
				)
				// Continue with other events instead of failing the whole batch
				continue
			}
			totalEventsPublished++

			// Update the last seen ID and timestamp for next batch
			lastID = event.ID
			lastTimestamp = event.Timestamp
		}

		s.Logger.Infow("published events for reprocessing for feature usage tracking",
			"batch", processedBatches,
			"count", eventsCount,
			"total_published", totalEventsPublished,
		)

		// Update for next batch
		processedBatches++

		// If we didn't get a full batch, we're done
		if eventsCount < batchSize {
			break
		}
	}

	s.Logger.Infow("completed event reprocessing for feature usage tracking",
		"external_customer_id", params.ExternalCustomerID,
		"event_name", params.EventName,
		"batches_processed", processedBatches,
		"total_events_found", totalEventsFound,
		"total_events_published", totalEventsPublished,
	)

	return nil
}
