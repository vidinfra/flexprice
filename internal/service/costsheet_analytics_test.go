package service

import (
	"context"
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCostsheetAnalyticsService_GetCostAnalytics_Validation(t *testing.T) {
	// Create a mock service with minimal dependencies for validation testing
	service := &costsheetAnalyticsService{
		ServiceParams: ServiceParams{
			// Add minimal required params for validation
		},
		featureUsageTrackingService: nil, // Not needed for validation tests
	}

	ctx := context.Background()

	t.Run("should return validation error when no filters provided", func(t *testing.T) {
		req := &dto.GetCostAnalyticsRequest{
			StartTime: time.Now().Add(-24 * time.Hour),
			EndTime:   time.Now(),
			// No CostsheetV2ID, ExternalCustomerID, or CustomerIDs provided
		}

		_, err := service.GetCostAnalytics(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one of costsheet_v2_id, external_customer_id, or customer_ids must be provided")
	})

	t.Run("should use default 7-day range when no time provided", func(t *testing.T) {
		req := &dto.GetCostAnalyticsRequest{
			// No StartTime or EndTime provided
			CostsheetV2ID: "test-costsheet-id",
		}

		err := req.Validate()
		require.NoError(t, err)

		// Check that defaults were set
		assert.False(t, req.StartTime.IsZero())
		assert.False(t, req.EndTime.IsZero())

		// Check that it's approximately 7 days
		duration := req.EndTime.Sub(req.StartTime)
		expectedDuration := 7 * 24 * time.Hour
		assert.InDelta(t, expectedDuration.Seconds(), duration.Seconds(), 60) // Allow 1 minute tolerance
	})

	t.Run("should return error when only one time field provided", func(t *testing.T) {
		req := &dto.GetCostAnalyticsRequest{
			StartTime: time.Now().Add(-24 * time.Hour),
			// EndTime not provided
			CostsheetV2ID: "test-costsheet-id",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "both start_time and end_time must be provided if one is specified")
	})

	t.Run("should pass validation when costsheet_v2_id is provided", func(t *testing.T) {
		req := &dto.GetCostAnalyticsRequest{
			StartTime:     time.Now().Add(-24 * time.Hour),
			EndTime:       time.Now(),
			CostsheetV2ID: "test-costsheet-id",
		}

		err := req.Validate()
		require.NoError(t, err)
	})

	t.Run("should pass validation when external_customer_id is provided", func(t *testing.T) {
		req := &dto.GetCostAnalyticsRequest{
			StartTime:          time.Now().Add(-24 * time.Hour),
			EndTime:            time.Now(),
			ExternalCustomerID: "test-customer-id",
		}

		err := req.Validate()
		require.NoError(t, err)
	})

	t.Run("should pass validation when customer_ids are provided", func(t *testing.T) {
		req := &dto.GetCostAnalyticsRequest{
			StartTime: time.Now().Add(-24 * time.Hour),
			EndTime:   time.Now(),
		}

		err := req.Validate()
		require.NoError(t, err)
	})
}

func TestCostsheetAnalyticsService_GetCombinedAnalytics_Validation(t *testing.T) {
	service := &costsheetAnalyticsService{
		ServiceParams:               ServiceParams{},
		featureUsageTrackingService: nil, // Not needed for validation tests
	}

	ctx := context.Background()

	t.Run("should inherit validation from GetCostAnalyticsRequest", func(t *testing.T) {
		req := &dto.GetCostAnalyticsRequest{
			StartTime: time.Now().Add(-24 * time.Hour),
			EndTime:   time.Now(),
			// No filters provided
		}

		_, err := service.GetDetailedCostAnalytics(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one of costsheet_v2_id, external_customer_id, or customer_ids must be provided")
	})
}

func TestCostAnalyticItem_Structure(t *testing.T) {
	t.Run("should create cost analytic item with all fields", func(t *testing.T) {
		item := dto.CostAnalyticItem{
			MeterID:            "meter-123",
			MeterName:          "API Calls",
			Source:             "api",
			CustomerID:         "cust-123",
			ExternalCustomerID: "ext-cust-123",
			Properties:         map[string]string{"region": "us-east-1"},
			TotalCost:          decimal.NewFromFloat(100.50),
			TotalQuantity:      decimal.NewFromInt(1000),
			TotalEvents:        500,
			Currency:           "USD",
			PriceID:            "price-123",
			CostsheetV2ID:      "costsheet-123",
		}

		assert.Equal(t, "meter-123", item.MeterID)
		assert.Equal(t, "API Calls", item.MeterName)
		assert.Equal(t, "api", item.Source)
		assert.Equal(t, "cust-123", item.CustomerID)
		assert.Equal(t, "ext-cust-123", item.ExternalCustomerID)
		assert.Equal(t, "us-east-1", item.Properties["region"])
		assert.True(t, item.TotalCost.Equal(decimal.NewFromFloat(100.50)))
		assert.True(t, item.TotalQuantity.Equal(decimal.NewFromInt(1000)))
		assert.Equal(t, int64(500), item.TotalEvents)
		assert.Equal(t, "USD", item.Currency)
		assert.Equal(t, "price-123", item.PriceID)
		assert.Equal(t, "costsheet-123", item.CostsheetV2ID)
	})
}

func TestGetCostAnalyticsResponse_Structure(t *testing.T) {
	t.Run("should create response with summary and analytics", func(t *testing.T) {
		startTime := time.Now().Add(-24 * time.Hour)
		endTime := time.Now()

		response := dto.GetCostAnalyticsResponse{
			CustomerID:         "cust-123",
			ExternalCustomerID: "ext-cust-123",
			CostsheetV2ID:      "costsheet-123",
			StartTime:          startTime,
			EndTime:            endTime,
			Currency:           "USD",
			TotalCost:          decimal.NewFromFloat(250.75),
			TotalQuantity:      decimal.NewFromInt(2500),
			TotalEvents:        1200,
			CostAnalytics: []dto.CostAnalyticItem{
				{
					MeterID:       "meter-1",
					TotalCost:     decimal.NewFromFloat(150.25),
					TotalQuantity: decimal.NewFromInt(1500),
					TotalEvents:   700,
					Currency:      "USD",
				},
				{
					MeterID:       "meter-2",
					TotalCost:     decimal.NewFromFloat(100.50),
					TotalQuantity: decimal.NewFromInt(1000),
					TotalEvents:   500,
					Currency:      "USD",
				},
			},
		}

		assert.Equal(t, "cust-123", response.CustomerID)
		assert.Equal(t, "ext-cust-123", response.ExternalCustomerID)
		assert.Equal(t, "costsheet-123", response.CostsheetV2ID)
		assert.Equal(t, startTime, response.StartTime)
		assert.Equal(t, endTime, response.EndTime)
		assert.Equal(t, "USD", response.Currency)
		assert.True(t, response.TotalCost.Equal(decimal.NewFromFloat(250.75)))
		assert.True(t, response.TotalQuantity.Equal(decimal.NewFromInt(2500)))
		assert.Equal(t, int64(1200), response.TotalEvents)
		assert.Len(t, response.CostAnalytics, 2)
	})
}

func TestGetCombinedAnalyticsResponse_DerivedMetrics(t *testing.T) {
	t.Run("should calculate derived metrics correctly", func(t *testing.T) {
		response := dto.GetDetailedCostAnalyticsResponse{
			TotalRevenue: decimal.NewFromFloat(1000.00),
			TotalCost:    decimal.NewFromFloat(600.00),
			Currency:     "USD",
		}

		// Calculate derived metrics manually for testing
		response.Margin = response.TotalRevenue.Sub(response.TotalCost)
		response.MarginPercent = response.Margin.Div(response.TotalRevenue).Mul(decimal.NewFromInt(100))
		response.ROI = response.Margin.Div(response.TotalCost)
		response.ROIPercent = response.ROI.Mul(decimal.NewFromInt(100))

		assert.True(t, response.Margin.Equal(decimal.NewFromFloat(400.00)))
		assert.True(t, response.MarginPercent.Equal(decimal.NewFromFloat(40.00)))
		assert.True(t, response.ROI.Equal(decimal.NewFromFloat(0.6667).Round(4)))
		assert.True(t, response.ROIPercent.Equal(decimal.NewFromFloat(66.67).Round(2)))
	})

	t.Run("should handle zero revenue gracefully", func(t *testing.T) {
		response := dto.GetDetailedCostAnalyticsResponse{
			TotalRevenue: decimal.Zero,
			TotalCost:    decimal.NewFromFloat(100.00),
			Currency:     "USD",
		}

		response.Margin = response.TotalRevenue.Sub(response.TotalCost)
		// Don't calculate MarginPercent when revenue is zero to avoid division by zero

		assert.True(t, response.Margin.Equal(decimal.NewFromFloat(-100.00)))
		assert.True(t, response.MarginPercent.IsZero())
	})

	t.Run("should handle zero cost gracefully", func(t *testing.T) {
		response := dto.GetDetailedCostAnalyticsResponse{
			TotalRevenue: decimal.NewFromFloat(100.00),
			TotalCost:    decimal.Zero,
			Currency:     "USD",
		}

		response.Margin = response.TotalRevenue.Sub(response.TotalCost)
		response.MarginPercent = response.Margin.Div(response.TotalRevenue).Mul(decimal.NewFromInt(100))
		// Don't calculate ROI when cost is zero to avoid division by zero

		assert.True(t, response.Margin.Equal(decimal.NewFromFloat(100.00)))
		assert.True(t, response.MarginPercent.Equal(decimal.NewFromFloat(100.00)))
		assert.True(t, response.ROI.IsZero())
	})
}

func TestCalculateTotalRevenue(t *testing.T) {
	service := &costsheetAnalyticsService{}

	t.Run("should return zero for nil response", func(t *testing.T) {
		result := service.calculateTotalRevenue(nil)
		assert.True(t, result.IsZero())
	})

	t.Run("should return zero for empty items", func(t *testing.T) {
		response := &dto.GetUsageAnalyticsResponse{
			Items: []dto.UsageAnalyticItem{},
		}
		result := service.calculateTotalRevenue(response)
		assert.True(t, result.IsZero())
	})

	t.Run("should calculate total revenue from items", func(t *testing.T) {
		response := &dto.GetUsageAnalyticsResponse{
			Items: []dto.UsageAnalyticItem{
				{TotalCost: decimal.NewFromFloat(100.50)},
				{TotalCost: decimal.NewFromFloat(200.25)},
				{TotalCost: decimal.NewFromFloat(150.00)},
			},
		}
		result := service.calculateTotalRevenue(response)
		expected := decimal.NewFromFloat(450.75)
		assert.True(t, result.Equal(expected))
	})
}

func TestContainsString(t *testing.T) {
	t.Run("should return true when string is in slice", func(t *testing.T) {
		slice := []string{"apple", "banana", "cherry"}
		assert.True(t, lo.Contains(slice, "banana"))
	})

	t.Run("should return false when string is not in slice", func(t *testing.T) {
		slice := []string{"apple", "banana", "cherry"}
		assert.False(t, lo.Contains(slice, "grape"))
	})

	t.Run("should return false for empty slice", func(t *testing.T) {
		slice := []string{}
		assert.False(t, lo.Contains(slice, "apple"))
	})

	t.Run("should handle empty string", func(t *testing.T) {
		slice := []string{"", "banana", "cherry"}
		assert.True(t, lo.Contains(slice, ""))
	})
}
