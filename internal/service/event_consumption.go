package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/domain/events"
	ierr "github.com/flexprice/flexprice/internal/errors"
	pubsubRouter "github.com/flexprice/flexprice/internal/pubsub/router"
	"github.com/flexprice/flexprice/internal/sentry"
)

// EventConsumptionService handles consuming raw events from Kafka and inserting them into ClickHouse
type EventConsumptionService interface {
	// Register message handler with the router
	RegisterHandler(router *pubsubRouter.Router, subscriber message.Subscriber, cfg *config.Configuration)

	// Process a raw event payload (used for AWS Lambda and direct processing)
	ProcessRawEvent(ctx context.Context, payload []byte) error
}

type eventConsumptionService struct {
	ServiceParams
	eventRepo              events.Repository
	sentryService          *sentry.Service
	eventPostProcessingSvc EventPostProcessingService
}

// NewEventConsumptionService creates a new event consumption service
func NewEventConsumptionService(
	params ServiceParams,
	eventRepo events.Repository,
	sentryService *sentry.Service,
	eventPostProcessingSvc EventPostProcessingService,
) EventConsumptionService {
	return &eventConsumptionService{
		ServiceParams:          params,
		eventRepo:              eventRepo,
		sentryService:          sentryService,
		eventPostProcessingSvc: eventPostProcessingSvc,
	}
}

// RegisterHandler registers the event consumption handler with the router
func (s *eventConsumptionService) RegisterHandler(
	router *pubsubRouter.Router,
	subscriber message.Subscriber,
	cfg *config.Configuration,
) {

	// throttle := middleware.NewThrottle(cfg.EventPostProcessing.RateLimit)
	router.AddNoPublishHandler(
		"event_consumption_handler",
		cfg.Kafka.Topic,
		subscriber,
		s.processMessage,
	)

	s.Logger.Infow("registered event consumption handler",
		"topic", cfg.Kafka.Topic,
		"consumer_group", cfg.Kafka.ConsumerGroup,
	)
}

// processMessage processes a single event message from Kafka
func (s *eventConsumptionService) processMessage(msg *message.Message) error {
	ctx := context.Background()

	// Start a transaction for this event processing
	transaction, ctx := s.sentryService.StartTransaction(ctx, "event.process")
	if transaction != nil {
		defer transaction.Finish()
	}

	// Start a Kafka consumer span
	kafkaSpan, ctx := s.sentryService.StartKafkaConsumerSpan(ctx, msg.Metadata.Get("topic"))
	if kafkaSpan != nil {
		defer kafkaSpan.Finish()
	}

	// Unmarshal the event
	var event events.Event
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		s.Logger.Errorw("failed to unmarshal event",
			"error", err,
			"payload", string(msg.Payload),
		)
		s.sentryService.CaptureException(err)

		// Return error for non-retriable parse errors
		// Watermill's poison queue middleware will handle moving it to DLQ
		if !shouldRetryError(err) {
			return fmt.Errorf("non-retriable unmarshal error: %w", err)
		}
		return err
	}

	s.Logger.Debugw("processing event",
		"event_id", event.ID,
		"event_name", event.EventName,
		"tenant_id", event.TenantID,
		"timestamp", event.Timestamp,
	)

	// Prepare events to insert
	eventsToInsert := []*events.Event{&event}

	// Create billing event if configured
	if s.Config.Billing.TenantID != "" {
		billingEvent := events.NewEvent(
			"tenant_event", // Standardized event name for billing
			s.Config.Billing.TenantID,
			event.TenantID, // Use original tenant ID as external customer ID
			map[string]interface{}{
				"original_event_id":   event.ID,
				"original_event_name": event.EventName,
				"original_timestamp":  event.Timestamp,
				"tenant_id":           event.TenantID,
				"source":              event.Source,
			},
			time.Now(),
			"", // Customer ID will be looked up by external ID
			"", // Generate new ID
			"system",
			s.Config.Billing.EnvironmentID,
		)
		eventsToInsert = append(eventsToInsert, billingEvent)
	}

	// Insert events into ClickHouse
	if err := s.eventRepo.BulkInsertEvents(ctx, eventsToInsert); err != nil {
		s.Logger.Errorw("failed to insert events",
			"error", err,
			"event_id", event.ID,
			"event_name", event.EventName,
		)

		// Return error for retry
		return ierr.WithError(err).
			WithHint("Failed to insert events into ClickHouse").
			Mark(ierr.ErrSystem)
	}

	// Publish event to post-processing service
	if err := s.eventPostProcessingSvc.PublishEvent(ctx, &event, false); err != nil {
		s.Logger.Errorw("failed to publish event to post-processing service",
			"error", err,
			"event_id", event.ID,
			"event_name", event.EventName,
		)

		// Return error for retry
		return ierr.WithError(err).
			WithHint("Failed to publish event for post-processing").
			Mark(ierr.ErrSystem)
	}

	s.Logger.Debugw("successfully processed event",
		"event_id", event.ID,
		"event_name", event.EventName,
		"lag_ms", time.Since(event.Timestamp).Milliseconds(),
	)

	return nil
}

// ProcessRawEvent processes a raw event payload (used for AWS Lambda and direct processing)
func (s *eventConsumptionService) ProcessRawEvent(ctx context.Context, payload []byte) error {
	// Start a transaction for this event processing
	transaction, ctx := s.sentryService.StartTransaction(ctx, "event.process")
	if transaction != nil {
		defer transaction.Finish()
	}

	// Unmarshal the event
	var event events.Event
	if err := json.Unmarshal(payload, &event); err != nil {
		s.Logger.Errorw("failed to unmarshal event",
			"error", err,
			"payload", string(payload),
		)
		s.sentryService.CaptureException(err)
		return fmt.Errorf("failed to unmarshal event: %w", err)
	}

	s.Logger.Debugw("processing raw event",
		"event_id", event.ID,
		"event_name", event.EventName,
		"tenant_id", event.TenantID,
		"timestamp", event.Timestamp,
	)

	// Prepare events to insert
	eventsToInsert := []*events.Event{&event}

	// Create billing event if configured
	if s.Config.Billing.TenantID != "" {
		billingEvent := events.NewEvent(
			"tenant_event", // Standardized event name for billing
			s.Config.Billing.TenantID,
			event.TenantID, // Use original tenant ID as external customer ID
			map[string]interface{}{
				"original_event_id":   event.ID,
				"original_event_name": event.EventName,
				"original_timestamp":  event.Timestamp,
				"tenant_id":           event.TenantID,
				"source":              event.Source,
			},
			time.Now(),
			"", // Customer ID will be looked up by external ID
			"", // Generate new ID
			"system",
			s.Config.Billing.EnvironmentID,
		)
		eventsToInsert = append(eventsToInsert, billingEvent)
	}

	// Insert events into ClickHouse
	if err := s.eventRepo.BulkInsertEvents(ctx, eventsToInsert); err != nil {
		s.Logger.Errorw("failed to insert events",
			"error", err,
			"event_id", event.ID,
			"event_name", event.EventName,
		)
		return fmt.Errorf("failed to insert events: %w", err)
	}

	// Publish event to post-processing service
	if err := s.eventPostProcessingSvc.PublishEvent(ctx, &event, false); err != nil {
		s.Logger.Errorw("failed to publish event to post-processing service",
			"error", err,
			"event_id", event.ID,
			"event_name", event.EventName,
		)
		return fmt.Errorf("failed to publish event for post-processing: %w", err)
	}

	s.Logger.Debugw("successfully processed raw event",
		"event_id", event.ID,
		"event_name", event.EventName,
		"lag_ms", time.Since(event.Timestamp).Milliseconds(),
	)

	return nil
}

// shouldRetryError determines if an error should trigger a message retry
func shouldRetryError(err error) bool {
	// Don't retry parsing errors which are not likely to succeed on retry
	errMsg := err.Error()
	if strings.Contains(errMsg, "unmarshal") ||
		strings.Contains(errMsg, "parse") ||
		strings.Contains(errMsg, "invalid") {
		return false
	}

	// Retry all other errors (database issues, network issues, etc.)
	return true
}
