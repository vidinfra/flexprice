package postgres

import (
	"context"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/internal/logger"
	sentryService "github.com/flexprice/flexprice/internal/sentry"
)

// SentryClient wraps the standard postgres client with Sentry monitoring
type SentryClient struct {
	client IClient
	sentry *sentryService.Service
	logger *logger.Logger
}

// NewSentryClient creates a new Sentry-instrumented Postgres client
func NewSentryClient(client IClient, sentry *sentryService.Service, logger *logger.Logger) IClient {
	return &SentryClient{
		client: client,
		sentry: sentry,
		logger: logger,
	}
}

// WithTx wraps the given function in a transaction with Sentry span tracking
func (c *SentryClient) WithTx(ctx context.Context, fn func(context.Context) error) error {
	span, spanCtx := c.sentry.StartDBSpan(ctx, "postgres.transaction", map[string]interface{}{
		"operation": "transaction",
		"target":    "writer", // Transactions always go to writer
	})
	if span != nil {
		defer span.Finish()
	}

	// Use the original client's WithTx but with the new span context
	return c.client.WithTx(spanCtx, fn)
}

// TxFromContext returns the transaction from context if it exists
func (c *SentryClient) TxFromContext(ctx context.Context) *ent.Tx {
	return c.client.TxFromContext(ctx)
}

// Writer returns the writer client for write operations
func (c *SentryClient) Writer(ctx context.Context) *ent.Client {
	return c.client.Writer(ctx)
}

// Reader returns the appropriate client for read operations
func (c *SentryClient) Reader(ctx context.Context) *ent.Client {
	return c.client.Reader(ctx)
}

// Close closes the database connection
func (c *SentryClient) Close() error {
	return c.client.Close()
}
