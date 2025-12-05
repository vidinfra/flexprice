package service

import (
	"context"
	"encoding/json"
	"fmt"
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

const (
	// Event sources for tracking where alerts originated
	EventSourceWalletCredit    = "wallet_credit"
	EventSourceWalletDebit     = "wallet_debit"
	EventSourceManualDebit     = "manual_debit"
	EventSourceCreditPurchase  = "credit_purchase"
	EventSourceCreditExpiry    = "credit_expiry"
	EventSourceWalletTerminate = "wallet_terminate"
	EventSourceCron            = "cron"
	EventSourceAPI             = "api"
)

// WalletBalanceAlertService handles wallet balance alert operations via Kafka
type WalletBalanceAlertService interface {
	// PublishEvent publishes a wallet balance alert event to Kafka
	PublishEvent(ctx context.Context, event *wallet.WalletBalanceAlertEvent) error

	// RegisterHandler registers the Kafka consumer handler
	RegisterHandler(router *pubsubRouter.Router, cfg *config.Configuration)
}

type walletBalanceAlertService struct {
	ServiceParams
	pubSub pubsub.PubSub
}

// NewWalletAlertsService creates a new wallet alerts service
func NewWalletBalanceAlertService(
	params ServiceParams,
) WalletBalanceAlertService {
	svc := &walletBalanceAlertService{
		ServiceParams: params,
	}

	// Initialize Kafka PubSub with dedicated consumer group
	pubSub, err := kafka.NewPubSubFromConfig(
		params.Config,
		params.Logger,
		params.Config.WalletAlerts.ConsumerGroup,
	)

	if err != nil {
		params.Logger.Fatalw("failed to create pubsub for wallet alerts",
			"error", err,
			"consumer_group", params.Config.WalletAlerts.ConsumerGroup,
		)
		return nil
	}
	svc.pubSub = pubSub

	params.Logger.Infow("wallet alerts service initialized",
		"topic", params.Config.WalletAlerts.Topic,
		"consumer_group", params.Config.WalletAlerts.ConsumerGroup,
		"rate_limit", params.Config.WalletAlerts.RateLimit,
	)

	return svc
}

// PublishEvent publishes a wallet balance alert event to Kafka
func (s *walletBalanceAlertService) PublishEvent(ctx context.Context, event *wallet.WalletBalanceAlertEvent) error {

	err := event.Validate()
	if err != nil {
		return ierr.WithError(err).
			WithHint("Invalid wallet balance alert event").
			WithReportableDetails(map[string]interface{}{
				"event_id":       event.ID,
				"customer_id":    event.CustomerID,
				"tenant_id":      event.TenantID,
				"environment_id": event.EnvironmentID,
			}).
			Mark(ierr.ErrValidation)
	}

	// Ensure event has ID and timestamp
	if event.ID == "" {
		event.ID = types.GenerateUUIDWithPrefix(types.UUID_PREFIX_WALLET_ALERT)
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	// Marshal event payload
	payload, err := json.Marshal(event)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to marshal wallet balance alert event").
			WithReportableDetails(map[string]interface{}{
				"event_id":       event.ID,
				"customer_id":    event.CustomerID,
				"tenant_id":      event.TenantID,
				"environment_id": event.EnvironmentID,
			}).
			Mark(ierr.ErrValidation)
	}

	// Create deterministic partition key for consistent routing
	// Events for the same customer always go to the same partition
	partitionKey := s.createPartitionKey(event)

	// Create Watermill message with unique ID
	msg := message.NewMessage(event.ID, payload)

	// Get topic from config
	topic := s.Config.WalletAlerts.Topic

	// Validate PubSub is initialized
	if s.pubSub == nil {
		return ierr.NewError("pubsub not initialized for wallet alerts").
			WithHint("Kafka PubSub failed to initialize during service creation").
			WithReportableDetails(map[string]interface{}{
				"topic":          topic,
				"consumer_group": s.Config.WalletAlerts.ConsumerGroup,
			}).
			Mark(ierr.ErrSystem)
	}

	s.Logger.Infow("publishing wallet balance alert event",
		"event_id", event.ID,
		"customer_id", event.CustomerID,
		"tenant_id", event.TenantID,
		"environment_id", event.EnvironmentID,
		"wallet_id", event.WalletID,
		"partition_key", partitionKey,
		"topic", topic,
		"source", event.Source,
		"force_calculate", event.ForceCalculateBalance,
	)

	// Publish to Kafka
	if err := s.pubSub.Publish(ctx, topic, msg); err != nil {
		return ierr.WithError(err).
			WithHint("Failed to publish wallet balance alert event to Kafka").
			WithReportableDetails(map[string]interface{}{
				"event_id":       event.ID,
				"customer_id":    event.CustomerID,
				"tenant_id":      event.TenantID,
				"environment_id": event.EnvironmentID,
				"topic":          topic,
			}).
			Mark(ierr.ErrSystem)
	}

	s.Logger.Debugw("wallet balance alert event published successfully",
		"event_id", event.ID,
		"customer_id", event.CustomerID,
	)

	return nil
}

// RegisterHandler registers a Kafka consumer handler with rate limiting
func (s *walletBalanceAlertService) RegisterHandler(router *pubsubRouter.Router, cfg *config.Configuration) {
	// Add throttle middleware for rate limiting
	throttle := middleware.NewThrottle(cfg.WalletAlerts.RateLimit, time.Second)

	// Register the handler
	router.AddNoPublishHandler(
		"wallet_balance_alert_handler",
		cfg.WalletAlerts.Topic,
		s.pubSub,
		s.processMessage,
		throttle.Middleware,
	)

	s.Logger.Infow("registered wallet balance alert handler",
		"handler_name", "wallet_balance_alert_handler",
		"topic", cfg.WalletAlerts.Topic,
		"consumer_group", cfg.WalletAlerts.ConsumerGroup,
		"rate_limit", cfg.WalletAlerts.RateLimit,
	)
}

// processMessage processes a single Kafka message for wallet balance alerts
func (s *walletBalanceAlertService) processMessage(msg *message.Message) error {
	startTime := time.Now()

	var event wallet.WalletBalanceAlertEvent
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		s.Logger.Errorw("failed to unmarshal wallet balance alert event",
			"error", err,
			"message_uuid", msg.UUID,
			"payload_size", len(msg.Payload),
		)
		return nil
	}
	s.Logger.Infow("processing wallet balance alert message",
		"message_uuid", msg.UUID,
		"tenant_id", event.TenantID,
		"environment_id", event.EnvironmentID,
		"customer_id", event.CustomerID,
		"event_source", event.Source,
		"published_at", event.Timestamp,
	)

	// Create context with tenant and environment IDs
	ctx := context.Background()
	if event.TenantID != "" {
		ctx = context.WithValue(ctx, types.CtxTenantID, event.TenantID)
	}
	if event.EnvironmentID != "" {
		ctx = context.WithValue(ctx, types.CtxEnvironmentID, event.EnvironmentID)
	}

	// Process the event
	if err := s.processEvent(ctx, event); err != nil {
		processingDuration := time.Since(startTime)
		s.Logger.Errorw("failed to process wallet balance alert event",
			"error", err,
			"event_id", event.ID,
			"customer_id", event.CustomerID,
			"tenant_id", event.TenantID,
			"environment_id", event.EnvironmentID,
			"wallet_id", event.WalletID,
			"source", event.Source,
			"processing_duration_ms", processingDuration.Milliseconds(),
		)
		// Return error to trigger retry with backoff
		return err
	}

	processingDuration := time.Since(startTime)
	s.Logger.Infow("wallet balance alert event processed successfully",
		"event_id", event.ID,
		"customer_id", event.CustomerID,
		"tenant_id", event.TenantID,
		"environment_id", event.EnvironmentID,
		"wallet_id", event.WalletID,
		"source", event.Source,
		"processing_duration_ms", processingDuration.Milliseconds(),
	)

	return nil
}

// processEvent delegates to the wallet service to check balance alerts
func (s *walletBalanceAlertService) processEvent(ctx context.Context, event wallet.WalletBalanceAlertEvent) error {
	// Create wallet service instance
	walletService := NewWalletService(s.ServiceParams)

	// Delegate to wallet service for actual processing
	return walletService.CheckWalletBalanceAlert(ctx, &event)
}

// createPartitionKey creates a deterministic partition key for Kafka
// This ensures all events for the same customer go to the same partition
func (s *walletBalanceAlertService) createPartitionKey(event *wallet.WalletBalanceAlertEvent) string {
	// Use tenant:environment:customer as partition key
	// This balances load while ensuring consistency per customer
	return fmt.Sprintf("%s:%s:%s", event.TenantID, event.EnvironmentID, event.CustomerID)
}
