package hubspot

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/domain/customer"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// DealSyncService handles synchronization of subscription data with HubSpot deals
type DealSyncService struct {
	client           HubSpotClient
	customerRepo     customer.Repository
	subscriptionRepo subscription.Repository
	priceRepo        price.Repository
	logger           *logger.Logger
}

// NewDealSyncService creates a new HubSpot deal sync service
func NewDealSyncService(
	client HubSpotClient,
	customerRepo customer.Repository,
	subscriptionRepo subscription.Repository,
	priceRepo price.Repository,
	logger *logger.Logger,
) *DealSyncService {
	return &DealSyncService{
		client:           client,
		customerRepo:     customerRepo,
		subscriptionRepo: subscriptionRepo,
		priceRepo:        priceRepo,
		logger:           logger,
	}
}

// SyncSubscriptionToDeal creates HubSpot line items from subscription and associates them with a deal
func (s *DealSyncService) SyncSubscriptionToDeal(ctx context.Context, subscriptionID string) error {
	s.logger.Infow("fetching subscription for deal sync",
		"subscription_id", subscriptionID)

	// Fetch subscription with line items
	sub, lineItems, err := s.subscriptionRepo.GetWithLineItems(ctx, subscriptionID)
	if err != nil {
		s.logger.Errorw("failed to fetch subscription with line items",
			"error", err,
			"subscription_id", subscriptionID)
		return ierr.WithError(err).
			WithHint("Failed to fetch subscription").
			Mark(ierr.ErrInternal)
	}

	// Assign line items to the subscription
	sub.LineItems = lineItems

	s.logger.Infow("fetching customer for deal sync",
		"customer_id", sub.CustomerID,
		"subscription_id", subscriptionID,
		"line_items_count", len(lineItems))

	// Fetch customer to get deal ID from metadata
	cust, err := s.customerRepo.Get(ctx, sub.CustomerID)
	if err != nil {
		s.logger.Errorw("failed to fetch customer",
			"error", err,
			"customer_id", sub.CustomerID,
			"subscription_id", subscriptionID)
		return ierr.WithError(err).
			WithHint("Failed to fetch customer").
			Mark(ierr.ErrInternal)
	}

	// Get deal ID from customer metadata
	dealID, ok := cust.Metadata["hubspot_deal_id"]
	if !ok || dealID == "" {
		s.logger.Warnw("no HubSpot deal ID found in customer metadata",
			"customer_id", cust.ID,
			"subscription_id", subscriptionID,
			"metadata", cust.Metadata)
		return nil // Not an error - customer might not be from HubSpot
	}

	s.logger.Infow("found HubSpot deal ID, creating line items",
		"deal_id", dealID,
		"subscription_id", subscriptionID,
		"line_items_count", len(sub.LineItems))

	// Filter for FIXED pricing only (flat rate)
	var flatRateLineItems []*subscription.SubscriptionLineItem
	for _, lineItem := range sub.LineItems {
		if lineItem.PriceType == "FIXED" {
			flatRateLineItems = append(flatRateLineItems, lineItem)
		}
	}

	if len(flatRateLineItems) == 0 {
		s.logger.Warnw("no flat rate line items to sync",
			"subscription_id", subscriptionID,
			"deal_id", dealID,
			"line_items_count", len(sub.LineItems))
		return nil
	}

	// Create HubSpot line items for each flat rate subscription line item
	for _, lineItem := range flatRateLineItems {
		if err := s.createHubSpotLineItem(ctx, lineItem, sub, dealID); err != nil {
			s.logger.Errorw("failed to create HubSpot line item",
				"error", err,
				"line_item_id", lineItem.ID,
				"deal_id", dealID)
			// Continue with other line items even if one fails
			continue
		}
	}

	s.logger.Infow("successfully synced subscription line items to HubSpot deal",
		"subscription_id", subscriptionID,
		"deal_id", dealID,
		"synced_items", len(flatRateLineItems))

	// Wait for HubSpot to recalculate ACV after line items are added
	s.logger.Infow("waiting for HubSpot to recalculate ACV",
		"deal_id", dealID,
		"wait_seconds", 5)
	time.Sleep(5 * time.Second)

	// Update deal amount based on ACV
	if err := s.updateDealAmount(ctx, dealID, sub); err != nil {
		s.logger.Errorw("failed to update deal amount",
			"error", err,
			"deal_id", dealID,
			"subscription_id", subscriptionID)
		// Don't return error - line items were created successfully
	}

	return nil
}

// createHubSpotLineItem creates a single HubSpot line item and associates it with a deal
func (s *DealSyncService) createHubSpotLineItem(
	ctx context.Context,
	lineItem *subscription.SubscriptionLineItem,
	sub *subscription.Subscription,
	dealID string,
) error {
	// Fetch the price to get the actual amount
	priceObj, err := s.priceRepo.Get(ctx, lineItem.PriceID)
	if err != nil {
		s.logger.Errorw("failed to fetch price for line item",
			"error", err,
			"price_id", lineItem.PriceID,
			"line_item_id", lineItem.ID)
		// Use a default price if we can't fetch it
		priceObj = &price.Price{
			Amount: decimal.NewFromInt(10), // Default fallback
		}
	}

	// Calculate unit price and total amount
	unitPrice := priceObj.Amount
	totalAmount := unitPrice.Mul(lineItem.Quantity)

	// Calculate discount (default to 0 if not applicable)
	discount := "0"

	// Map billing frequency to HubSpot format
	billingFreq := s.mapBillingFrequency(sub.BillingPeriod)

	// Build description with pricing model
	description := string(lineItem.PriceType) + " pricing"
	if lineItem.DisplayName != "" {
		description = lineItem.DisplayName + " (" + string(lineItem.PriceType) + " pricing)"
	}

	// Create line item request with all fields
	lineItemReq := &DealLineItemCreateRequest{
		Properties: DealLineItemProperties{
			Name:                 lineItem.DisplayName,
			Price:                unitPrice.String(),         // Unit price
			Quantity:             lineItem.Quantity.String(), // Quantity
			Amount:               totalAmount.String(),       // Total amount
			Discount:             discount,                   // Discount
			RecurringBillingFreq: billingFreq,                // Billing frequency
			Description:          description,                // Pricing model in description
		},
		Associations: []LineItemAssociation{
			{
				To: AssociationTarget{
					ID: dealID,
				},
				Types: []AssociationType{
					{
						AssociationCategory: "HUBSPOT_DEFINED",
						AssociationTypeID:   20, // Line item to deal association
					},
				},
			},
		},
	}

	s.logger.Infow("creating HubSpot line item",
		"deal_id", dealID,
		"line_item_name", lineItem.DisplayName,
		"quantity", lineItem.Quantity.String(),
		"unit_price", unitPrice.String(),
		"total_amount", totalAmount.String(),
		"discount", discount,
		"billing_frequency", billingFreq,
		"pricing_model", lineItem.PriceType)

	// Create the line item in HubSpot
	_, err = s.client.CreateDealLineItem(ctx, lineItemReq)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to create HubSpot line item").
			Mark(ierr.ErrHTTPClient)
	}

	return nil
}

// mapBillingFrequency converts FlexPrice billing period to HubSpot billing frequency
func (s *DealSyncService) mapBillingFrequency(period types.BillingPeriod) string {
	switch period {
	case types.BILLING_PERIOD_MONTHLY:
		return "monthly"
	case types.BILLING_PERIOD_ANNUAL:
		return "annually"
	case types.BILLING_PERIOD_WEEKLY:
		return "weekly"
	case types.BILLING_PERIOD_QUARTER:
		return "quarterly"
	default:
		return string(period)
	}
}

// updateDealAmount fetches the deal's ACV and updates the deal amount to match it
func (s *DealSyncService) updateDealAmount(ctx context.Context, dealID string, sub *subscription.Subscription) error {
	s.logger.Infow("fetching deal to get ACV",
		"deal_id", dealID,
		"subscription_id", sub.ID)

	// Get the deal to read its ACV
	deal, err := s.client.GetDeal(ctx, dealID)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to fetch deal from HubSpot").
			Mark(ierr.ErrHTTPClient)
	}

	s.logger.Infow("fetched deal properties",
		"deal_id", dealID,
		"properties", deal.Properties)

	// Extract ACV from deal properties
	var acv string
	if deal.Properties != nil {
		// Try different possible property names and types
		if acvValue, ok := deal.Properties["hs_acv"].(string); ok && acvValue != "" {
			acv = acvValue
			s.logger.Infow("found ACV as string", "deal_id", dealID, "acv", acv)
		} else if acvValue, ok := deal.Properties["hs_acv"].(float64); ok {
			acv = decimal.NewFromFloat(acvValue).String()
			s.logger.Infow("found ACV as float64", "deal_id", dealID, "acv", acv)
		} else {
			s.logger.Warnw("hs_acv property not found or empty",
				"deal_id", dealID,
				"available_properties", deal.Properties)
		}
	}

	// If no ACV found, calculate from subscription
	if acv == "" {
		s.logger.Warnw("no ACV found in deal, calculating from subscription",
			"deal_id", dealID,
			"subscription_id", sub.ID)

		// Calculate annual value from subscription
		totalAmount := decimal.Zero
		for _, lineItem := range sub.LineItems {
			if lineItem.PriceType == "FIXED" {
				priceObj, err := s.priceRepo.Get(ctx, lineItem.PriceID)
				if err == nil {
					totalAmount = totalAmount.Add(priceObj.Amount.Mul(lineItem.Quantity))
				}
			}
		}

		// Convert to annual based on billing period
		switch sub.BillingPeriod {
		case types.BILLING_PERIOD_MONTHLY:
			totalAmount = totalAmount.Mul(decimal.NewFromInt(12))
		case types.BILLING_PERIOD_QUARTER:
			totalAmount = totalAmount.Mul(decimal.NewFromInt(4))
		case types.BILLING_PERIOD_WEEKLY:
			totalAmount = totalAmount.Mul(decimal.NewFromInt(52))
		}

		acv = totalAmount.String()
	}

	s.logger.Infow("updating deal amount with ACV",
		"deal_id", dealID,
		"acv", acv,
		"subscription_id", sub.ID)

	// Update the deal amount
	updateProps := map[string]string{
		"amount": acv,
	}

	_, err = s.client.UpdateDeal(ctx, dealID, updateProps)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to update deal amount").
			Mark(ierr.ErrHTTPClient)
	}

	s.logger.Infow("successfully updated deal amount",
		"deal_id", dealID,
		"amount", acv,
		"subscription_id", sub.ID)

	return nil
}
