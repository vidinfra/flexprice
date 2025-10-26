package dto

import (
	"time"

	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/flexprice/flexprice/internal/validator"
	"github.com/shopspring/decimal"
)

// GetCostAnalyticsRequest represents the request to get cost analytics
type GetCostAnalyticsRequest struct {
	// Time range fields (optional - defaults to last 7 days if not provided)
	StartTime time.Time `json:"start_time,omitempty"`
	EndTime   time.Time `json:"end_time,omitempty"`

	// Optional filters - at least one of these should be provided
	CostsheetV2ID      string `json:"costsheet_v2_id,omitempty"`      // Optional - for specific costsheet
	ExternalCustomerID string `json:"external_customer_id,omitempty"` // Optional - for specific customer

	// Additional filters
	MeterIDs        []string            `json:"meter_ids,omitempty"`
	Sources         []string            `json:"sources,omitempty"`
	WindowSize      types.WindowSize    `json:"window_size,omitempty"` // For time-series
	GroupBy         []string            `json:"group_by,omitempty"`    // "meter_id", "source", "customer_id"
	PropertyFilters map[string][]string `json:"property_filters,omitempty"`

	// Additional options
	IncludeTimeSeries bool `json:"include_time_series,omitempty"`
	IncludeBreakdown  bool `json:"include_breakdown,omitempty"`

	// Pagination
	Limit  int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`
}

// Validate validates the cost analytics request and sets defaults
func (r *GetCostAnalyticsRequest) Validate() error {
	if err := validator.ValidateRequest(r); err != nil {
		return err
	}

	// Set default time range to last 7 days if not provided
	if r.StartTime.IsZero() && r.EndTime.IsZero() {
		now := time.Now().UTC()
		r.EndTime = now
		r.StartTime = now.Add(-7 * 24 * time.Hour)
	} else if r.StartTime.IsZero() || r.EndTime.IsZero() {
		return ierr.NewError("both start_time and end_time must be provided if one is specified").
			WithHint("Please provide both start_time and end_time, or omit both for default 7-day range").
			Mark(ierr.ErrValidation)
	}

	if r.CostsheetV2ID == "" {
		return ierr.NewError("costsheet_v2_id is required").
			WithHint("costsheet_v2_id is required").
			Mark(ierr.ErrValidation)
	}

	return nil
}

// GetCombinedAnalyticsRequest represents the request to get combined cost and revenue analytics
type GetCombinedAnalyticsRequest struct {
	GetCostAnalyticsRequest
	// Revenue analytics options
	IncludeRevenue bool `json:"include_revenue"`
}

// Validate validates the combined analytics request
func (r *GetCombinedAnalyticsRequest) Validate() error {
	return r.GetCostAnalyticsRequest.Validate()
}

// CostAnalyticItem represents a single cost analytics item
type CostAnalyticItem struct {
	MeterID            string            `json:"meter_id"`
	MeterName          string            `json:"meter_name,omitempty"`
	Source             string            `json:"source,omitempty"`
	CustomerID         string            `json:"customer_id,omitempty"`
	ExternalCustomerID string            `json:"external_customer_id,omitempty"`
	Properties         map[string]string `json:"properties,omitempty"`

	// Aggregated metrics
	TotalCost     decimal.Decimal `json:"total_cost"`
	TotalQuantity decimal.Decimal `json:"total_quantity"`
	TotalEvents   int64           `json:"total_events"`

	// Breakdown
	CostByPeriod []CostPoint `json:"cost_by_period,omitempty"` // Time-series

	// Metadata
	Currency      string `json:"currency"`
	PriceID       string `json:"price_id,omitempty"`
	CostsheetV2ID string `json:"costsheet_v2_id,omitempty"`
}

// CostPoint represents a single point in cost time-series data
type CostPoint struct {
	Timestamp  time.Time       `json:"timestamp"`
	Cost       decimal.Decimal `json:"cost"`
	Quantity   decimal.Decimal `json:"quantity"`
	EventCount int64           `json:"event_count"`
}

// GetCostAnalyticsResponse represents the response for cost analytics
type GetCostAnalyticsResponse struct {
	CustomerID         string    `json:"customer_id,omitempty"`
	ExternalCustomerID string    `json:"external_customer_id,omitempty"`
	CostsheetV2ID      string    `json:"costsheet_v2_id,omitempty"`
	StartTime          time.Time `json:"start_time"`
	EndTime            time.Time `json:"end_time"`
	Currency           string    `json:"currency"`

	// Summary
	TotalCost     decimal.Decimal `json:"total_cost"`
	TotalQuantity decimal.Decimal `json:"total_quantity"`
	TotalEvents   int64           `json:"total_events"`

	// Detailed breakdown
	CostAnalytics []CostAnalyticItem `json:"cost_analytics"`

	// Time-series (if requested)
	CostTimeSeries []CostPoint `json:"cost_time_series,omitempty"`

	// Pagination
	Pagination *types.PaginationResponse `json:"pagination,omitempty"`
}

// GetCombinedAnalyticsResponse represents the response for combined cost and revenue analytics
type GetCombinedAnalyticsResponse struct {
	// Cost metrics
	CostAnalytics *GetCostAnalyticsResponse `json:"cost_analytics"`

	// Revenue metrics (from existing analytics)
	RevenueAnalytics *GetUsageAnalyticsResponse `json:"revenue_analytics,omitempty"`

	// Derived metrics
	TotalRevenue  decimal.Decimal `json:"total_revenue"`
	TotalCost     decimal.Decimal `json:"total_cost"`
	Margin        decimal.Decimal `json:"margin"`         // Revenue - Cost
	MarginPercent decimal.Decimal `json:"margin_percent"` // (Margin / Revenue) * 100
	ROI           decimal.Decimal `json:"roi"`            // (Revenue - Cost) / Cost
	ROIPercent    decimal.Decimal `json:"roi_percent"`    // ROI * 100

	Currency  string    `json:"currency"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}
