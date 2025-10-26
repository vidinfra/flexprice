package service

import (
	"context"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/customer"
	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/domain/price"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/interfaces"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

type CostsheetAnalyticsService = interfaces.CostsheetAnalyticsService

type costsheetAnalyticsService struct {
	ServiceParams
	featureUsageTrackingService FeatureUsageTrackingV2Service
}

func NewCostsheetAnalyticsService(params ServiceParams, featureUsageTrackingService FeatureUsageTrackingV2Service) CostsheetAnalyticsService {
	return &costsheetAnalyticsService{
		ServiceParams:               params,
		featureUsageTrackingService: featureUsageTrackingService,
	}
}

// GetCostAnalytics retrieves cost analytics following the GetUsageBySubscription pattern
func (s *costsheetAnalyticsService) GetCostAnalytics(
	ctx context.Context,
	req *dto.GetCostAnalyticsRequest,
) (*dto.GetCostAnalyticsResponse, error) {
	// 1. Validate request
	if err := req.Validate(); err != nil {
		return nil, ierr.WithError(err).
			WithHint("Invalid cost analytics request").
			Mark(ierr.ErrValidation)
	}

	// 2. Fetch costsheet and associated prices (like subscription line items)
	prices, err := s.fetchCostsheetPrices(ctx, req.CostsheetV2ID)
	if err != nil {
		return nil, err
	}

	if len(prices) == 0 {
		s.Logger.Warnw("no prices found for costsheet", "costsheet_v2_id", req.CostsheetV2ID)
		return s.buildEmptyResponse(req), nil
	}

	// 3. Get customers based on request filters
	customers, err := s.fetchCustomers(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(customers) == 0 {
		s.Logger.Warnw("no customers found for request filters")
		return s.buildEmptyResponse(req), nil
	}

	// 4. Build meter usage requests for each customer-price combination
	meterUsageRequests := s.buildMeterUsageRequests(ctx, customers, prices, req)

	if len(meterUsageRequests) == 0 {
		s.Logger.Warnw("no meter usage requests generated")
		return s.buildEmptyResponse(req), nil
	}

	// 5. Use existing BulkGetUsageByMeter (same as subscription billing)
	eventService := NewEventService(s.EventRepo, s.MeterRepo, s.EventPublisher, s.Logger, s.Config)
	usageMap, err := eventService.BulkGetUsageByMeter(ctx, meterUsageRequests)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to get usage data").
			Mark(ierr.ErrInternal)
	}

	// 6. Calculate costs using price service (same logic as subscription)
	costAnalytics := s.calculateCostsFromUsage(ctx, usageMap, prices, customers, req)

	// 7. Optionally add time-series data
	if req.IncludeTimeSeries {
		costAnalytics = s.addTimeSeriesData(ctx, req, costAnalytics)
	}

	// 8. Build response
	return s.buildResponse(req, costAnalytics), nil
}

// GetCombinedAnalytics combines cost and revenue analytics with derived metrics
func (s *costsheetAnalyticsService) GetCombinedAnalytics(
	ctx context.Context,
	req *dto.GetCombinedAnalyticsRequest,
) (*dto.GetCombinedAnalyticsResponse, error) {
	// 1. Fetch cost analytics
	costAnalytics, err := s.GetCostAnalytics(ctx, &req.GetCostAnalyticsRequest)
	if err != nil {
		return nil, err
	}

	// 2. Fetch revenue analytics (use existing service) if requested
	var revenueAnalytics *dto.GetUsageAnalyticsResponse
	if req.IncludeRevenue {
		revenueReq := s.buildRevenueRequest(req)
		if revenueReq != nil {
			revenueAnalytics, err = s.featureUsageTrackingService.GetDetailedUsageAnalytics(ctx, revenueReq)
			if err != nil {
				s.Logger.Warnw("failed to fetch revenue analytics", "error", err)
				// Continue without revenue data - don't fail the entire request
				revenueAnalytics = nil
			}
		}
	}

	// 3. Compute derived metrics
	response := &dto.GetCombinedAnalyticsResponse{
		CostAnalytics:    costAnalytics,
		RevenueAnalytics: revenueAnalytics,
		TotalCost:        costAnalytics.TotalCost,
		Currency:         costAnalytics.Currency,
		StartTime:        costAnalytics.StartTime,
		EndTime:          costAnalytics.EndTime,
	}

	// Calculate derived metrics if revenue analytics is available
	// Note: revenueAnalytics will be nil until revenue integration is implemented
	response.TotalRevenue = s.calculateTotalRevenue(revenueAnalytics)
	response.Margin = response.TotalRevenue.Sub(response.TotalCost)

	if !response.TotalRevenue.IsZero() {
		response.MarginPercent = response.Margin.Div(response.TotalRevenue).Mul(decimal.NewFromInt(100))
	}

	if !response.TotalCost.IsZero() {
		response.ROI = response.Margin.Div(response.TotalCost)
		response.ROIPercent = response.ROI.Mul(decimal.NewFromInt(100))
	}

	return response, nil
}

// fetchCostsheetPrices fetches prices associated with a costsheet
func (s *costsheetAnalyticsService) fetchCostsheetPrices(ctx context.Context, costsheetV2ID string) ([]*price.Price, error) {
	if costsheetV2ID == "" {
		// If no costsheet specified, we could fetch all prices, but for now return empty
		return []*price.Price{}, nil
	}

	priceService := NewPriceService(s.ServiceParams)
	priceFilter := types.NewNoLimitPriceFilter()
	priceFilter.EntityIDs = []string{costsheetV2ID}
	priceFilter.EntityType = lo.ToPtr(types.PRICE_ENTITY_TYPE_COSTSHEET_V2)
	priceFilter.Status = lo.ToPtr(types.StatusPublished)
	priceFilter.Expand = lo.ToPtr(string(types.ExpandMeters))

	pricesResponse, err := priceService.GetPrices(ctx, priceFilter)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to fetch costsheet prices").
			Mark(ierr.ErrDatabase)
	}

	// Convert to domain models
	prices := make([]*price.Price, len(pricesResponse.Items))
	for i, priceResp := range pricesResponse.Items {
		prices[i] = priceResp.Price
	}

	return prices, nil
}

// fetchCustomers gets customers based on request filters
func (s *costsheetAnalyticsService) fetchCustomers(ctx context.Context, req *dto.GetCostAnalyticsRequest) ([]*customer.Customer, error) {

	// If specific external customer ID is provided
	if req.ExternalCustomerID != "" {
		customerFilter := types.NewCustomerFilter()
		customerFilter.ExternalID = req.ExternalCustomerID
		customersList, err := s.CustomerRepo.List(ctx, customerFilter)
		if err != nil {
			return nil, ierr.WithError(err).
				WithHint("Failed to fetch customer").
				Mark(ierr.ErrDatabase)
		}
		if len(customersList) == 0 {
			return []*customer.Customer{}, nil // Return empty instead of error
		}
		return customersList, nil
	}

	// If no specific customers, fetch all customers (with pagination)
	customerFilter := types.NewCustomerFilter()
	if req.Limit > 0 {
		customerFilter.Limit = lo.ToPtr(req.Limit)
	}
	if req.Offset > 0 {
		customerFilter.Offset = lo.ToPtr(req.Offset)
	}

	customersResponse, err := s.CustomerRepo.List(ctx, customerFilter)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to fetch customers").
			Mark(ierr.ErrDatabase)
	}

	return customersResponse, nil
}

// buildMeterUsageRequests creates usage requests for each customer-price combination
func (s *costsheetAnalyticsService) buildMeterUsageRequests(
	ctx context.Context,
	customers []*customer.Customer,
	prices []*price.Price,
	req *dto.GetCostAnalyticsRequest,
) []*dto.GetUsageByMeterRequest {
	var meterUsageRequests []*dto.GetUsageByMeterRequest

	// For each customer and each price, create a usage request
	for _, customer := range customers {
		for _, price := range prices {
			// Skip if meter ID is not set
			if price.MeterID == "" {
				continue
			}

			// Apply meter ID filter if specified
			if len(req.MeterIDs) > 0 && !containsString(req.MeterIDs, price.MeterID) {
				continue
			}

			usageRequest := &dto.GetUsageByMeterRequest{
				MeterID:            price.MeterID,
				PriceID:            price.ID,
				ExternalCustomerID: customer.ExternalID,
				StartTime:          req.StartTime,
				EndTime:            req.EndTime,
				Filters:            make(map[string][]string),
			}

			// Apply property filters if specified
			for key, values := range req.PropertyFilters {
				usageRequest.Filters[key] = values
			}

			meterUsageRequests = append(meterUsageRequests, usageRequest)
		}
	}

	return meterUsageRequests
}

// calculateCostsFromUsage processes usage results and calculates costs
func (s *costsheetAnalyticsService) calculateCostsFromUsage(
	ctx context.Context,
	usageMap map[string]*events.AggregationResult,
	prices []*price.Price,
	customers []*customer.Customer,
	req *dto.GetCostAnalyticsRequest,
) []dto.CostAnalyticItem {
	priceService := NewPriceService(s.ServiceParams)
	var costAnalytics []dto.CostAnalyticItem

	// Build price map for quick lookup
	priceMap := make(map[string]*price.Price)
	for _, p := range prices {
		priceMap[p.ID] = p
	}

	// Build customer map for quick lookup
	customerMap := make(map[string]*customer.Customer)
	for _, c := range customers {
		customerMap[c.ExternalID] = c
	}

	// Process each usage result
	for priceID, usage := range usageMap {
		price, exists := priceMap[priceID]
		if !exists {
			continue
		}

		// Find customer for this usage - we need to get it from the request since AggregationResult doesn't have ExternalCustomerID
		// For now, we'll use the first customer if there's only one, otherwise skip
		if len(customers) != 1 {
			continue // Skip if we can't determine which customer this usage belongs to
		}
		customer := customers[0]

		// Calculate cost (same logic as GetUsageBySubscription)
		var cost decimal.Decimal
		var quantity decimal.Decimal

		// Handle bucketed max meters
		if usage.MeterID != "" {
			meter, err := s.MeterRepo.GetMeter(ctx, usage.MeterID)
			if err == nil && meter.IsBucketedMaxMeter() {
				// For bucketed max, use array of values
				bucketedValues := make([]decimal.Decimal, len(usage.Results))
				for i, result := range usage.Results {
					bucketedValues[i] = result.Value
				}
				cost = priceService.CalculateBucketedCost(ctx, price, bucketedValues)

				// Calculate quantity as sum of all bucket maxes
				quantity = decimal.Zero
				for _, bucketValue := range bucketedValues {
					quantity = quantity.Add(bucketValue)
				}
			} else {
				// For all other cases, use single value
				quantity = usage.Value
				cost = priceService.CalculateCost(ctx, price, quantity)
			}
		} else {
			quantity = usage.Value
			cost = priceService.CalculateCost(ctx, price, quantity)
		}

		costAnalytic := dto.CostAnalyticItem{
			MeterID:            usage.MeterID,
			MeterName:          "", // Will be enriched later
			Source:             "", // Can be extracted from usage if needed
			CustomerID:         customer.ID,
			ExternalCustomerID: customer.ExternalID,
			TotalCost:          cost,
			TotalQuantity:      quantity,
			TotalEvents:        int64(len(usage.Results)),
			Currency:           price.Currency,
			PriceID:            price.ID,
			CostsheetV2ID:      price.EntityID, // Assuming price is linked to costsheet
		}

		costAnalytics = append(costAnalytics, costAnalytic)
	}

	return costAnalytics
}

// addTimeSeriesData adds time-series data to cost analytics (placeholder for now)
func (s *costsheetAnalyticsService) addTimeSeriesData(
	ctx context.Context,
	req *dto.GetCostAnalyticsRequest,
	costAnalytics []dto.CostAnalyticItem,
) []dto.CostAnalyticItem {
	// TODO: Implement time-series data fetching
	// This would involve querying ClickHouse with time windows
	// For now, return as-is
	return costAnalytics
}

// buildResponse builds the final cost analytics response
func (s *costsheetAnalyticsService) buildResponse(
	req *dto.GetCostAnalyticsRequest,
	costAnalytics []dto.CostAnalyticItem,
) *dto.GetCostAnalyticsResponse {
	response := &dto.GetCostAnalyticsResponse{
		CostsheetV2ID: req.CostsheetV2ID,
		StartTime:     req.StartTime,
		EndTime:       req.EndTime,
		CostAnalytics: costAnalytics,
		TotalCost:     decimal.Zero,
		TotalQuantity: decimal.Zero,
		TotalEvents:   0,
	}

	// Set customer info if single customer
	if req.ExternalCustomerID != "" {
		response.ExternalCustomerID = req.ExternalCustomerID
	}

	// Calculate totals
	var currency string
	for _, item := range costAnalytics {
		response.TotalCost = response.TotalCost.Add(item.TotalCost)
		response.TotalQuantity = response.TotalQuantity.Add(item.TotalQuantity)
		response.TotalEvents += item.TotalEvents
		if currency == "" {
			currency = item.Currency
		}
	}
	response.Currency = currency

	return response
}

// buildEmptyResponse builds an empty response for cases with no data
func (s *costsheetAnalyticsService) buildEmptyResponse(req *dto.GetCostAnalyticsRequest) *dto.GetCostAnalyticsResponse {
	return &dto.GetCostAnalyticsResponse{
		CostsheetV2ID:      req.CostsheetV2ID,
		ExternalCustomerID: req.ExternalCustomerID,
		StartTime:          req.StartTime,
		EndTime:            req.EndTime,
		Currency:           "USD", // Default currency
		TotalCost:          decimal.Zero,
		TotalQuantity:      decimal.Zero,
		TotalEvents:        0,
		CostAnalytics:      []dto.CostAnalyticItem{},
	}
}

// buildRevenueRequest builds a revenue analytics request from combined request
func (s *costsheetAnalyticsService) buildRevenueRequest(req *dto.GetCombinedAnalyticsRequest) *dto.GetUsageAnalyticsRequest {
	return &dto.GetUsageAnalyticsRequest{
		ExternalCustomerID: req.ExternalCustomerID,
		StartTime:          req.StartTime,
		EndTime:            req.EndTime,
		WindowSize:         req.WindowSize,
		GroupBy:            req.GroupBy,
	}
}

// calculateTotalRevenue calculates total revenue from usage analytics response
func (s *costsheetAnalyticsService) calculateTotalRevenue(revenueAnalytics *dto.GetUsageAnalyticsResponse) decimal.Decimal {
	if revenueAnalytics == nil {
		return decimal.Zero
	}

	totalRevenue := decimal.Zero
	for _, item := range revenueAnalytics.Items {
		totalRevenue = totalRevenue.Add(item.TotalCost) // TotalCost in usage analytics represents revenue
	}
	return totalRevenue
}

// containsString checks if a string slice contains a specific string
func containsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
