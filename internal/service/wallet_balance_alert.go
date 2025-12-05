package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/message/router/middleware"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/domain/wallet"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/pubsub"
	"github.com/flexprice/flexprice/internal/pubsub/kafka"
	pubsubRouter "github.com/flexprice/flexprice/internal/pubsub/router"
	"github.com/flexprice/flexprice/internal/types"
)

// FeatureUsageTrackingService handles feature usage tracking operations for metered events
type WalletBalanceAlertService interface {
	// Publish an event for wallet alerts
	PublishEvent(ctx context.Context, event *wallet.WalletBalanceAlertEvent) error

	// Register Handler for this
	RegisterHandler(router *pubsubRouter.Router, cfg *config.Configuration)
}

type walletBalanceAlertService struct {
	ServiceParams
	pubSub pubsub.PubSub // Regular PubSub for normal processing
}

// NewWalletAlertsService creates a new wallet alerts service
func NewWalletAlertsService(
	params ServiceParams,
) WalletBalanceAlertService {
	ev := &walletBalanceAlertService{
		ServiceParams: params,
	}

	pubSub, err := kafka.NewPubSubFromConfig(
		params.Config,
		params.Logger,
		params.Config.WalletAlerts.ConsumerGroup,
	)

	if err != nil {
		params.Logger.Fatalw("failed to create pubsub", "error", err)
		return nil
	}
	ev.pubSub = pubSub

	return ev
}

// PublishEvent publishes an event to the feature usage tracking topic
func (s *walletBalanceAlertService) PublishEvent(ctx context.Context, event *wallet.WalletBalanceAlertEvent) error {
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
	if event.CustomerID != "" {
		partitionKey = fmt.Sprintf("%s:%s", event.TenantID, event.CustomerID)
	}

	uuid := types.GenerateUUID()
	// Make UUID truly unique by adding nanosecond precision timestamp and random bytes
	uniqueID := fmt.Sprintf("%s-%d-%d", uuid, time.Now().UnixNano(), rand.Int63())

	// Use the partition key as the message ID to ensure consistent partitioning
	msg := message.NewMessage(uniqueID, payload)

	// Set metadata for additional context
	msg.Metadata.Set("tenant_id", event.TenantID)
	msg.Metadata.Set("environment_id", event.EnvironmentID)
	msg.Metadata.Set("partition_key", partitionKey)

	pubSub := s.pubSub
	topic := s.Config.WalletAlerts.Topic

	if pubSub == nil {
		return ierr.NewError("pubsub not initialized").
			WithHint("Please check the config").
			Mark(ierr.ErrSystem)
	}

	s.Logger.Debugw("publishing event for wallet balance alert",
		"event_id", uuid,
		"partition_key", partitionKey,
		"topic", topic,
	)

	// Publish to wallet balance alert topic using the PubSub (Kafka)
	if err := pubSub.Publish(ctx, topic, msg); err != nil {
		return ierr.WithError(err).
			WithHint("Failed to publish event for wallet balance alert").
			Mark(ierr.ErrSystem)
	}
	return nil
}

// RegisterHandler registers a handler for the wallet balance alert topic with rate limiting
func (s *walletBalanceAlertService) RegisterHandler(router *pubsubRouter.Router, cfg *config.Configuration) {
	// Add throttle middleware to this specific handler
	throttle := middleware.NewThrottle(cfg.WalletAlerts.RateLimit, time.Second)

	// Add the handler
	router.AddNoPublishHandler(
		"wallet_balance_alert_handler",
		cfg.WalletAlerts.Topic,
		s.pubSub,
		s.processMessage,
		throttle.Middleware,
	)

	s.Logger.Infow("registered event wallet balance alert handler",
		"topic", cfg.WalletAlerts.Topic,
		"rate_limit", cfg.WalletAlerts.RateLimit,
	)

}

// Process a single event message for wallet balance alert
func (s *walletBalanceAlertService) processMessage(msg *message.Message) error {
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
	var event wallet.WalletBalanceAlertEvent
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		s.Logger.Errorw("failed to unmarshal event for wallet balance alert",
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
	if err := s.processEvent(ctx, event); err != nil {
		s.Logger.Errorw("failed to process event for wallet balance alert",
			"error", err,
			"customer_id", event.CustomerID,
			"tenant_id", event.TenantID,
			"environment_id", event.EnvironmentID,
		)
		return err // Return error for retry
	}

	s.Logger.Infow("event for wallet balance alert processed successfully",
		"customer_id", event.CustomerID,
		"tenant_id", event.TenantID,
		"environment_id", event.EnvironmentID,
	)

	return nil
}

// Process a single event for feature usage tracking
func (s *walletBalanceAlertService) processEvent(ctx context.Context, event wallet.WalletBalanceAlertEvent) error {
	walletService := NewWalletService(s.ServiceParams)

	return walletService.CheckWalletBalanceAlert(ctx, &event)
}
