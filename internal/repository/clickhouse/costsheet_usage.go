package clickhouse

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/flexprice/flexprice/internal/clickhouse"
	"github.com/flexprice/flexprice/internal/domain/events"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

type CostSheetUsageRepository struct {
	store  *clickhouse.ClickHouseStore
	logger *logger.Logger
}

func NewCostSheetUsageRepository(store *clickhouse.ClickHouseStore, logger *logger.Logger) events.CostSheetUsageRepository {
	return &CostSheetUsageRepository{
		store:  store,
		logger: logger,
	}
}

// InsertProcessedEvent inserts a single processed event
func (r *CostSheetUsageRepository) InsertProcessedEvent(ctx context.Context, event *events.CostUsage) error {
	query := `
		INSERT INTO costsheet_usage (
			id, tenant_id, external_customer_id, customer_id, event_name, source, 
			timestamp, ingested_at, properties, environment_id,
			costsheet_id, price_id, meter_id, feature_id, currency,
			unique_hash, qty_total, sign
		) VALUES (
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
		)
	`

	propertiesJSON, err := json.Marshal(event.Properties)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to marshal event properties").
			WithReportableDetails(map[string]interface{}{
				"event_id": event.ID,
			}).
			Mark(ierr.ErrValidation)
	}

	// Default sign to 1 if not set
	sign := event.Sign
	if sign == 0 {
		sign = 1
	}

	args := []interface{}{
		event.ID,
		event.TenantID,
		event.ExternalCustomerID,
		event.CustomerID,
		event.EventName,
		event.Source,
		event.Timestamp,
		event.IngestedAt,
		string(propertiesJSON),
		event.EnvironmentID,
		event.CostSheetID,
		event.PriceID,
		event.MeterID,
		event.FeatureID,
		event.UniqueHash,
		event.QtyTotal,
		sign,
	}

	err = r.store.GetConn().Exec(ctx, query, args...)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to insert processed event").
			WithReportableDetails(map[string]interface{}{
				"event_id": event.ID,
			}).
			Mark(ierr.ErrDatabase)
	}

	return nil
}

// BulkInsertProcessedEvents inserts multiple processed events
func (r *CostSheetUsageRepository) BulkInsertProcessedEvents(ctx context.Context, events []*events.CostUsage) error {
	if len(events) == 0 {
		return nil
	}

	// Split events in batches of 100
	eventsBatches := lo.Chunk(events, 100)

	for _, eventsBatch := range eventsBatches {
		// Prepare batch statement
		batch, err := r.store.GetConn().PrepareBatch(ctx, `
			INSERT INTO costsheet_usage (
				id, tenant_id, external_customer_id, customer_id, event_name, source, 
				timestamp, ingested_at, properties, environment_id,
				costsheet_id, price_id, meter_id, feature_id,
				unique_hash, qty_total, sign
			)
		`)
		if err != nil {
			return ierr.WithError(err).
				WithHint("Failed to prepare batch for feature usage").
				Mark(ierr.ErrDatabase)
		}

		for _, event := range eventsBatch {
			propertiesJSON, err := json.Marshal(event.Properties)
			if err != nil {
				return ierr.WithError(err).
					WithHint("Failed to marshal event properties").
					WithReportableDetails(map[string]interface{}{
						"event_id": event.ID,
					}).
					Mark(ierr.ErrValidation)
			}

			// Default sign to 1 if not set
			sign := event.Sign
			if sign == 0 {
				sign = 1
			}

			err = batch.Append(
				event.ID,
				event.TenantID,
				event.ExternalCustomerID,
				event.CustomerID,
				event.EventName,
				event.Source,
				event.Timestamp,
				event.IngestedAt,
				string(propertiesJSON),
				event.EnvironmentID,
				event.CostSheetID,
				event.PriceID,
				event.MeterID,
				event.FeatureID,
				event.UniqueHash,
				event.QtyTotal,
				sign,
			)

			if err != nil {
				return ierr.WithError(err).
					WithHint("Failed to append cost usage to batch").
					WithReportableDetails(map[string]interface{}{
						"event_id": event.ID,
					}).
					Mark(ierr.ErrDatabase)
			}
		}

		// Send batch
		if err := batch.Send(); err != nil {
			return ierr.WithError(err).
				WithHint("Failed to execute batch insert for cost usage").
				WithReportableDetails(map[string]interface{}{
					"event_count": len(events),
				}).
				Mark(ierr.ErrDatabase)
		}
	}

	return nil
}

// GetProcessedEvents retrieves processed events based on the provided parameters
func (r *CostSheetUsageRepository) GetProcessedEvents(ctx context.Context, params *events.GetCostUsageEventsParams) ([]*events.CostUsage, uint64, error) {
	query := `
		SELECT 
			id, tenant_id, external_customer_id, customer_id, event_name, source, 
			timestamp, ingested_at, properties, processed_at, environment_id,
			costsheet_id, price_id, meter_id, feature_id,
			unique_hash, qty_total,version, sign, processing_lag_ms
		FROM costsheet_usage
		WHERE tenant_id = ?
		AND environment_id = ?
		AND timestamp >= ?
		AND timestamp <= ?
	`

	countQuery := `
		SELECT COUNT(*)
		FROM costsheet_usage
		WHERE tenant_id = ?
		AND environment_id = ?
		AND timestamp >= ?
		AND timestamp <= ?
	`

	args := []interface{}{types.GetTenantID(ctx), types.GetEnvironmentID(ctx), params.StartTime, params.EndTime}
	countArgs := []interface{}{types.GetTenantID(ctx), types.GetEnvironmentID(ctx), params.StartTime, params.EndTime}

	// Add filters
	if params.CustomerID != "" {
		query += " AND customer_id = ?"
		countQuery += " AND customer_id = ?"
		args = append(args, params.CustomerID)
		countArgs = append(countArgs, params.CustomerID)
	}

	if params.CostSheetID != "" {
		query += " AND costsheet_id = ?"
		countQuery += " AND costsheet_id = ?"
		args = append(args, params.CostSheetID)
		countArgs = append(countArgs, params.CostSheetID)
	}

	if params.MeterID != "" {
		query += " AND meter_id = ?"
		countQuery += " AND meter_id = ?"
		args = append(args, params.MeterID)
		countArgs = append(countArgs, params.MeterID)
	}

	if params.FeatureID != "" {
		query += " AND feature_id = ?"
		countQuery += " AND feature_id = ?"
		args = append(args, params.FeatureID)
		countArgs = append(countArgs, params.FeatureID)
	}

	if params.PriceID != "" {
		query += " AND price_id = ?"
		countQuery += " AND price_id = ?"
		args = append(args, params.PriceID)
		countArgs = append(countArgs, params.PriceID)
	}

	// Add FINAL modifier to handle ReplacingMergeTree
	query += " FINAL"
	countQuery += " FINAL"

	// Apply pagination
	if params.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, params.Limit)

		if params.Offset > 0 {
			query += " OFFSET ?"
			args = append(args, params.Offset)
		}
	}

	// Execute count query if requested
	var totalCount uint64 = 0
	if params.CountTotal {
		err := r.store.GetConn().QueryRow(ctx, countQuery, countArgs...).Scan(&totalCount)
		if err != nil {
			return nil, 0, ierr.WithError(err).
				WithHint("Failed to count processed events").
				Mark(ierr.ErrDatabase)
		}
	}

	// Execute main query
	rows, err := r.store.GetConn().Query(ctx, query, args...)
	if err != nil {
		return nil, 0, ierr.WithError(err).
			WithHint("Failed to query processed events").
			Mark(ierr.ErrDatabase)
	}
	defer rows.Close()

	var eventsList []*events.CostUsage
	for rows.Next() {
		var event events.CostUsage
		var propertiesJSON string

		err := rows.Scan(
			&event.ID,
			&event.TenantID,
			&event.ExternalCustomerID,
			&event.CustomerID,
			&event.EventName,
			&event.Source,
			&event.Timestamp,
			&event.IngestedAt,
			&propertiesJSON,
			&event.ProcessedAt,
			&event.EnvironmentID,
			&event.CostSheetID,
			&event.PriceID,
			&event.MeterID,
			&event.FeatureID,
			&event.UniqueHash,
			&event.QtyTotal,
			&event.Version,
			&event.Sign,
			&event.ProcessingLagMs,
		)
		if err != nil {
			return nil, 0, ierr.WithError(err).
				WithHint("Failed to scan processed event").
				Mark(ierr.ErrDatabase)
		}

		// Parse properties
		if propertiesJSON != "" {
			if err := json.Unmarshal([]byte(propertiesJSON), &event.Properties); err != nil {
				return nil, 0, ierr.WithError(err).
					WithHint("Failed to unmarshal properties").
					Mark(ierr.ErrValidation)
			}
		} else {
			event.Properties = make(map[string]interface{})
		}

		eventsList = append(eventsList, &event)
	}

	return eventsList, totalCount, nil
}

// GetFeatureUsageBySubscription gets usage data for a subscription using a single optimized query
func (r *CostSheetUsageRepository) GetUsageByCostSheetID(ctx context.Context, costSheetID, externalCustomerID string, startTime, endTime time.Time) (map[string]*events.UsageByCostSheetResult, error) {
	// Extract tenantID and environmentID from context
	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)

	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "costsheet_usage", "get_usage_by_costsheet_id", map[string]interface{}{
		"costsheet_id":         costSheetID,
		"external_customer_id": externalCustomerID,
		"environment_id":       environmentID,
		"tenant_id":            tenantID,
		"start_time":           startTime,
		"end_time":             endTime,
	})
	defer FinishSpan(span)

	query := `
		SELECT 
			costsheet_id,
			feature_id,
			meter_id,
			price_id,
			sum(qty_total * sign)              AS sum_total,
			max(qty_total * sign)              AS max_total,
			count(DISTINCT id)                 AS count_distinct_ids,
			count(DISTINCT unique_hash)        AS count_unique_qty,
			argMax(qty_total * sign, "timestamp") AS latest_qty
		FROM costsheet_usage
		WHERE 
			costsheet_id = ?
			AND external_customer_id = ?
			AND environment_id = ?
			AND tenant_id = ?
			AND "timestamp" >= ?
			AND "timestamp" < ?
			AND sign != 0
		GROUP BY costsheet_id, feature_id, meter_id, price_id
	`

	log.Printf("Executing query: %s", query)
	log.Printf("Params: %v", []interface{}{costSheetID, externalCustomerID, environmentID, tenantID, startTime, endTime})

	rows, err := r.store.GetConn().Query(ctx, query, costSheetID, externalCustomerID, environmentID, tenantID, startTime, endTime)
	if err != nil {
		SetSpanError(span, err)
		return nil, ierr.WithError(err).
			WithHint("Failed to execute optimized subscription usage query").
			WithReportableDetails(map[string]interface{}{
				"costsheet_id":         costSheetID,
				"external_customer_id": externalCustomerID,
			}).
			Mark(ierr.ErrDatabase)
	}
	defer rows.Close()

	results := make(map[string]*events.UsageByCostSheetResult)
	for rows.Next() {
		var costSheetID, featureID, meterID, priceID string
		var sumTotal, maxTotal, latestQty decimal.Decimal
		var countDistinctIDs, countUniqueQty uint64

		err := rows.Scan(&costSheetID, &featureID, &meterID, &priceID, &sumTotal, &maxTotal, &countDistinctIDs, &countUniqueQty, &latestQty)
		if err != nil {
			SetSpanError(span, err)
			return nil, ierr.WithError(err).
				WithHint("Failed to scan usage result").
				Mark(ierr.ErrDatabase)
		}

		results[costSheetID] = &events.UsageByCostSheetResult{
			CostSheetID: costSheetID,
			FeatureID:   featureID,
			MeterID:     meterID,
			PriceID:     priceID,
			SumTotal:    sumTotal,
			MaxTotal:    maxTotal,
			LatestQty:   latestQty,
		}
	}

	if err := rows.Err(); err != nil {
		SetSpanError(span, err)
		return nil, ierr.WithError(err).
			WithHint("Error iterating usage results").
			Mark(ierr.ErrDatabase)
	}

	SetSpanSuccess(span)
	r.logger.Debugw("optimized costsheet usage query completed",
		"costsheet_id", costSheetID,
		"feature_count", len(results))

	return results, nil
}

// formatWindowSize formats window size for ClickHouse queries
func (r *CostSheetUsageRepository) formatWindowSize(windowSize types.WindowSize, billingAnchor *time.Time) string {
	switch windowSize {
	case types.WindowSizeMinute:
		return "toStartOfMinute(timestamp)"
	case types.WindowSizeHour:
		return "toStartOfHour(timestamp)"
	case types.WindowSizeDay:
		return "toStartOfDay(timestamp)"
	case types.WindowSizeWeek:
		return "toStartOfWeek(timestamp)"
	case types.WindowSize15Min:
		return "toStartOfInterval(timestamp, INTERVAL 15 MINUTE)"
	case types.WindowSize30Min:
		return "toStartOfInterval(timestamp, INTERVAL 30 MINUTE)"
	case types.WindowSize3Hour:
		return "toStartOfInterval(timestamp, INTERVAL 3 HOUR)"
	case types.WindowSize6Hour:
		return "toStartOfInterval(timestamp, INTERVAL 6 HOUR)"
	case types.WindowSize12Hour:
		return "toStartOfInterval(timestamp, INTERVAL 12 HOUR)"
	case types.WindowSizeMonth:
		// Use custom monthly billing period if billing anchor is provided
		if billingAnchor != nil {
			// Extract only the day component from billing anchor for simplicity
			anchorDay := billingAnchor.Day()
			// Generate the custom monthly window expression using day-level granularity
			return fmt.Sprintf(`
				addDays(
					toStartOfMonth(addDays(timestamp, -%d)),
					%d
				)`, anchorDay-1, anchorDay-1)
		}
		return "toStartOfMonth(timestamp)"
	default:
		return "toStartOfHour(timestamp)"
	}
}
